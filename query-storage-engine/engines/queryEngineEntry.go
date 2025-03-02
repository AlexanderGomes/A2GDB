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
