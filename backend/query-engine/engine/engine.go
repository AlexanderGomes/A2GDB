package engine

import (
	"a2gdb/storage-engine/storage"
	"log"
)

type QueryEngine struct {
	BufferPoolManager *storage.BufferPoolManager
}

func (qe *QueryEngine) EngineEntry(queryPlan interface{}) ([]*storage.RowV2, map[string]int) {
	plan := queryPlan.(map[string]interface{})
	var rows []*storage.RowV2
	var groupByMap map[string]int

	switch operation := plan["STATEMENT"]; operation {
	case "CREATE_TABLE":
		qe.handleCreate(plan)
	case "INSERT":
		qe.handleInsert(plan)
	case "SELECT":
		rows, groupByMap = qe.handleSelect(plan)
	case "DELETE":
		qe.handleDelete(plan)
	case "UPDATE":
		qe.handleUpdate(plan)
	default:
		log.Panicf("Unsupported Type: %s", operation)
	}

	return rows, groupByMap
}

func (qe *QueryEngine) handleSelect(plan map[string]interface{}) ([]*storage.RowV2, map[string]int) {
	var rows []*storage.RowV2
	var selectedCols []interface{}
	var colName string
	var groupByMap map[string]int

	nodes := plan["rels"].([]interface{})
	referenceList := plan["refList"].(map[string]interface{})

	for _, node := range nodes {
		nodeInnerMap := node.(map[string]interface{})

		switch nodeOperation := nodeInnerMap["relOp"]; nodeOperation {
		case "LogicalTableScan":
			tableName := nodeInnerMap["table"].([]interface{})[0].(string)
			rows = storage.GetAllRows(tableName, qe.BufferPoolManager.DiskManager)
		case "LogicalProject":
			selectedCols, colName = columnSelect(nodeInnerMap, referenceList, rows)
		case "LogicalFilter":
			filterByColumn(nodeInnerMap, referenceList, &rows)
		case "LogicalSort":
			sortAscDesc(nodeInnerMap, &rows)
		case "LogicalAggregate":
			groupByMap = groupBy(nodeInnerMap, colName, &rows, selectedCols)
		default:
			log.Fatalf("Unsupported Type: %s", nodeOperation)
		}
	}

	return rows, groupByMap
}
