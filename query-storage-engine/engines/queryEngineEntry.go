package engines

import (
	"context"
	"fmt"
	"sync"
)

const (
	AUTH = iota + 1
	CREATE_TABLE
	QUERY
)

type QueryEngine struct {
	BufferPoolManager *BufferPoolManager
	Lm                *LockManager
	QueryChan         chan *QueryInfo
	ResChan           chan *Result
	InlineMu          sync.Mutex
}

type QueryInfo struct {
	RawPlan        interface{}
	tableName      string
	TransactionOff bool
	InduceErr      bool
}

func (qe *QueryEngine) QueryManager() {
	var result Result

	for queryInfo := range qe.QueryChan {
		queryPlan := queryInfo.RawPlan

		plan, isMap := queryPlan.(map[string]interface{})
		frontendErr, ok := plan["message"].(string)
		if ok || !isMap {
			result.Error = fmt.Errorf("frontend failed: %s", frontendErr)
			result.Msg = "failed"
			qe.ResChan <- &result
			continue
		}

		switch operation := plan["STATEMENT"]; operation {
		case "CREATE_TABLE", "SELECT":
			go func() {
				qe.ResChan <- qe.QueryProcessingEntry(queryPlan, queryInfo.TransactionOff, queryInfo.InduceErr)
			}()
		case "INSERT", "DELETE", "UPDATE":
			queryInfo.tableName = plan["table"].(string)
			go qe.InlineCruds(queryInfo)
		default:
			result.Error = fmt.Errorf("unsupported type: %s", operation)
			result.Msg = "failed"
			qe.ResChan <- &result
		}

	}
}

func (qe *QueryEngine) InlineCruds(queryInfo *QueryInfo) {
	qe.InlineMu.Lock()
	defer qe.InlineMu.Unlock()

	tablesMap := qe.BufferPoolManager.Wal.activeTxTable
	tableInfo, ok := tablesMap[queryInfo.tableName]
	if !ok {
		tableInfo = &Table{notification: make(chan bool, 1)}
		tablesMap[queryInfo.tableName] = tableInfo
	}

	if tableInfo.activeTx {
		for <-tableInfo.notification {
			qe.ResChan <- qe.QueryProcessingEntry(queryInfo.RawPlan, queryInfo.TransactionOff, queryInfo.InduceErr)
		}
		return
	}

	tableInfo.activeTx = true
	qe.ResChan <- qe.QueryProcessingEntry(queryInfo.RawPlan, queryInfo.TransactionOff, queryInfo.InduceErr)
}

func (qe *QueryEngine) QueryProcessingEntry(queryPlan interface{}, transactionOff, induceErr bool) *Result {
	var result Result

	plan, isMap := queryPlan.(map[string]interface{})
	frontendErr, ok := plan["message"].(string)
	if ok || !isMap {
		result.Error = fmt.Errorf("frontend failed: %s", frontendErr)
		result.Msg = "failed"
		return &result
	}

	switch operation := plan["STATEMENT"]; operation {
	case "CREATE_TABLE":
		result = qe.handleCreate(plan)
	case "INSERT":
		result = qe.handleInsert(plan, transactionOff, induceErr)
	case "SELECT":
		result = qe.handleSelect(plan)
	case "DELETE":
		result = qe.handleDelete(plan, transactionOff, induceErr)
	case "UPDATE":
		result = qe.handleUpdate(plan, transactionOff, induceErr)
	default:
		result.Error = fmt.Errorf("unsupported type: %s", operation)
		result.Msg = "failed"
	}

	return &result
}

func (qe *QueryEngine) handleSelect(plan map[string]interface{}) Result {
	var result Result

	nodes, err := ComputeNodes(plan, qe)
	if err != nil {
		return handleError(fmt.Errorf("ComputeNodes Failed: %w", err), "failed")
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(nodes))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, node := range nodes {
		wg.Add(1)
		go func(node Node) {
			defer wg.Done()
			if err := node.initialization(ctx); err != nil {
				errChan <- err
				cancel()
			}
		}(node)
	}

	wg.Wait()
	close(errChan)

	firstError := <-errChan
	if firstError != nil {
		return handleError(fmt.Errorf("handleSelect Failed: %w", firstError), "failed")
	}

	result.Rows = nodes[len(nodes)-1].GetRes()
	result.Msg = "success"

	return result
}
