package engine

import (
	"fmt"
)

type QueryEngine struct {
	BufferPoolManager *BufferPoolManager
}

func (qe *QueryEngine) EngineEntry(queryPlan interface{}, transactionOff bool) ([]*RowV2, map[string]int, *Result) {
	var rows []*RowV2
	var groupByMap map[string]int
	var result Result

	plan := queryPlan.(map[string]interface{})
	frontendErr, ok := plan["message"].(string)
	if ok {
		result.Error = fmt.Errorf("frontend failed: %s", frontendErr)
		result.Msg = "failed"
		return nil, nil, &result
	}

	switch operation := plan["STATEMENT"]; operation {
	case "CREATE_TABLE":
		result = qe.handleCreate(plan)
	case "INSERT":
		result = qe.handleInsert(plan)
	case "SELECT":
		rows, groupByMap, result = qe.handleSelect(plan)
	case "DELETE":
		result = qe.handleDelete(plan, transactionOff)
	case "UPDATE":
		result = qe.handleUpdate(plan, transactionOff, true)
	default:
		result.Error = fmt.Errorf("unsupported type: %s", operation)
		result.Msg = "failed"
	}

	return rows, groupByMap, &result
}

// ## return rows, and groupMap for test compatibility
// ## DBMS fundamentals could be applied, consider vector processing
func (qe *QueryEngine) handleSelect(plan map[string]interface{}) ([]*RowV2, map[string]int, Result) {
	var err error
	var rows []*RowV2
	var selectedCols []interface{}
	var colName string
	var groupByMap map[string]int
	var result Result

	nodes := plan["rels"].([]interface{})
	referenceList := plan["refList"].(map[string]interface{})
	for _, node := range nodes {
		nodeInnerMap := node.(map[string]interface{})

		switch nodeOperation := nodeInnerMap["relOp"]; nodeOperation {
		case "LogicalTableScan":
			tableName := nodeInnerMap["table"].([]interface{})[0].(string)
			rows, err = GetAllRows(tableName, qe.BufferPoolManager.DiskManager)
			if err != nil {
				result.Error = fmt.Errorf("LogicalTableScan - GetAllRows failed: %w", err)
				result.Msg = "failed"
				return nil, nil, result
			}
		case "LogicalProject":
			selectedCols, colName = columnSelect(nodeInnerMap, referenceList, rows)
		case "LogicalFilter":
			err := filterByColumn(nodeInnerMap, referenceList, &rows)
			if err != nil {
				result.Error = fmt.Errorf("LogicalFilter - filterByColumn failed: %w", err)
				result.Msg = "failed"
				return nil, nil, result
			}
		case "LogicalSort":
			sortAscDesc(nodeInnerMap, &rows)
		case "LogicalAggregate":
			groupByMap, err = groupBy(nodeInnerMap, colName, &rows, selectedCols)
			if err != nil {
				result.Error = fmt.Errorf("LogicalAggregate - groupBy failed: %w", err)
				result.Msg = "failed"
				return nil, nil, result
			}
			result.groupBy = groupByMap
		default:
			result.Error = fmt.Errorf("unsupported type: %s", nodeOperation)
			return nil, nil, result
		}
	}

	result.Rows = rows

	return rows, groupByMap, result
}
