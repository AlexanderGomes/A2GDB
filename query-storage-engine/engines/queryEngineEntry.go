package engines

import (
	"fmt"

	"github.com/scylladb/go-set/strset"
)

const (
	AUTH = iota + 1
	CREATE_TABLE
	QUERY
)

type QueryEngine struct {
	BufferPoolManager *BufferPoolManager
}

func (qe *QueryEngine) QueryProcessingEntry(queryPlan interface{}, transactionOff, induceErr bool) ([]*RowV2, map[string]int, *Result) {
	var rows []*RowV2
	var groupByMap map[string]int
	var result Result

	plan, isMap := queryPlan.(map[string]interface{})
	frontendErr, ok := plan["message"].(string)
	if ok || !isMap {
		result.Error = fmt.Errorf("frontend failed: %s", frontendErr)
		result.Msg = "failed"
		return nil, nil, &result
	}

	switch operation := plan["STATEMENT"]; operation {
	case "CREATE_TABLE":
		result = qe.handleCreate(plan)
	case "INSERT":
		result = qe.handleInsert(plan, transactionOff, induceErr)
	case "SELECT":
		groupByMap, result = qe.handleSelect(plan)
	case "DELETE":
		result = qe.handleDelete(plan, transactionOff, induceErr)
	case "UPDATE":
		result = qe.handleUpdate(plan, transactionOff, induceErr)
	default:
		result.Error = fmt.Errorf("unsupported type: %s", operation)
		result.Msg = "failed"
	}

	return rows, groupByMap, &result
}

// ## return rows, and groupMap for test compatibility
// ## DBMS fundamentals could be applied, consider vector processing
func (qe *QueryEngine) handleSelect(plan map[string]interface{}) (map[string]int, Result) {
	var result Result             // result type
	var groupByMap map[string]int // result type
	var selectedCols []interface{}
	var groupKey string
	var physicalNodes []Node
	var set *strset.Set

	logicalNodes := plan["rels"].([]interface{})
	referenceList := plan["refList"].(map[string]interface{})
	for _, node := range logicalNodes {
		nodeInnerMap := node.(map[string]interface{})

		switch nodeOperation := nodeInnerMap["relOp"]; nodeOperation {
		case "LogicalTableScan":
			tableName := nodeInnerMap["table"].([]interface{})[0].(string)
			scanNode := TableScanNode{
				TableName:  tableName,
				Dm:         qe.BufferPoolManager.DiskManager,
				OutputChan: make(chan []*RowV2),
			}

			physicalNodes = append(physicalNodes, scanNode)
		case "LogicalProject":
			selectedCols, groupKey, set = GetColInfo(nodeInnerMap, referenceList)
			projectNode := ProjectionNode{
				Set:        set,
				InputChan:  physicalNodes[len(physicalNodes)-1].GetOutputChan(),
				OutputChan: make(chan []*RowV2),
			}
			physicalNodes = append(physicalNodes, projectNode)
		case "LogicalFilter":
			filterNode := FilterNode{
				InnerMap:   nodeInnerMap,
				RefList:    referenceList,
				InputChan:  physicalNodes[len(physicalNodes)-1].GetOutputChan(),
				OutputChan: make(chan []*RowV2),
			}

			physicalNodes = append(physicalNodes, filterNode)
		case "LogicalSort":
			sortNode := SortNode{
				InnerMap:  nodeInnerMap,
				InputChan: physicalNodes[len(physicalNodes)-1].GetOutputChan(),
			}

			physicalNodes = append(physicalNodes, sortNode)
		case "LogicalAggregate":
			aggregateNode := AggregateNode{
				InnerMap:     nodeInnerMap,
				GroupKey:     groupKey,
				SelectedCols: selectedCols,
				InputChan:    physicalNodes[len(physicalNodes)-1].GetOutputChan(),
				OutputChan:   make(chan []*RowV2),
			}

			physicalNodes = append(physicalNodes, aggregateNode)
		default:
			return nil, handleError(fmt.Errorf("unsupported type: %s", nodeOperation), "failed")
		}
	}

	return groupByMap, result
}
