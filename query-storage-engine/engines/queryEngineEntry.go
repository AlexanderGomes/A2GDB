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
	ResultManager     *ResultManager
	Scheduler         *QueryScheduler
	InlineMu          sync.Mutex
}

type QueryInfo struct {
	Id             uint64
	Type           string
	RawPlan        interface{}
	tableName      string
	TransactionOff bool
	InduceErr      bool
}

func (qe *QueryEngine) QueryManager() {
	var result *Result
	var plan map[string]interface{}

	for queryInfo := range qe.QueryChan {
		queryPlan := queryInfo.RawPlan
		result, plan = unwrapPlannerInfo(queryPlan)

		switch operation := plan["STATEMENT"]; operation {
		case "CREATE_TABLE", "SELECT":
			queryInfo.Type = "NON_CRUD"
			qe.Scheduler.Queries <- queryInfo
		case "INSERT", "DELETE", "UPDATE":
			queryInfo.Type = "CRUD"
			queryInfo.tableName = plan["table"].(string)
			qe.Scheduler.Queries <- queryInfo
		default:
			result.Error = fmt.Errorf("unsupported type: %s", operation)
			result.Msg = "failed"
			qe.ResultManager.GlobalChannel <- result
		}

	}
}

func unwrapPlannerInfo(queryPlan interface{}) (*Result, map[string]interface{}) {
	var result Result

	plan, isMap := queryPlan.(map[string]interface{})
	frontendErr, ok := plan["message"].(string)
	if ok || !isMap {
		result.Error = fmt.Errorf("frontend failed: %s", frontendErr)
		result.Msg = "failed"
	}

	return &result, plan
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
			qe.ResultManager.GlobalChannel <- qe.QueryProcessingEntry(queryInfo)
		}
		return
	}

	tableInfo.activeTx = true
	qe.ResultManager.GlobalChannel <- qe.QueryProcessingEntry(queryInfo)
}

func (qe *QueryEngine) QueryProcessingEntry(queryInfo *QueryInfo) *Result {
	var result Result

	plan, isMap := queryInfo.RawPlan.(map[string]interface{})
	frontendErr, ok := plan["message"].(string)
	if ok || !isMap {
		result.Error = fmt.Errorf("frontend failed: %s", frontendErr)
		result.Msg = "failed"
		return &result
	}

	switch operation := plan["STATEMENT"]; operation {
	case "CREATE_TABLE":
		result = qe.handleCreate(plan)
		result.QueryTye = "NON_CRUD"
	case "INSERT":
		result = qe.handleInsert(plan, queryInfo.TransactionOff, queryInfo.InduceErr)
		result.QueryTye = "CRUD"
	case "SELECT":
		result = qe.handleSelect(plan)
		result.QueryTye = "NON_CRUD"
	case "DELETE":
		result = qe.handleDelete(plan, queryInfo.TransactionOff, queryInfo.InduceErr)
		result.QueryTye = "CRUD"
	case "UPDATE":
		result = qe.handleUpdate(plan, queryInfo.TransactionOff, queryInfo.InduceErr)
		result.QueryTye = "CRUD"
	default:
		result.Error = fmt.Errorf("unsupported type: %s", operation)
		result.Msg = "failed"
	}

	result.QueryId = queryInfo.Id

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
