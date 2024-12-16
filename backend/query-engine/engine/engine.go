package engine

import (
	"a2gdb/storage-engine/storage"
	"log"
)

type QueryEngine struct {
	StorageManager *storage.DiskManagerV2
}

func (qe *QueryEngine) EngineEntry(queryPlan interface{}) {
	plan := queryPlan.(map[string]interface{})

	switch operation := plan["STATEMENT"]; operation {
	case "CREATE_TABLE":
		qe.handleCreate(plan)
	case "INSERT":
		qe.handleInsert(plan)
	case "SELECT":
		qe.handleSelect(plan)
	case "DELETE":
		qe.handleDelete(plan)
	default:
		log.Panicf("Unsupported Type: %s", operation)
	}
}

func (qe *QueryEngine) handleSelect(plan map[string]interface{}) {
	var rows []*storage.RowV2
	var selectedCols []interface{}
	var colName string

	nodes := plan["rels"].([]interface{})
	referenceList := plan["refList"].(map[string]interface{})

	for _, node := range nodes {
		nodeInnerMap := node.(map[string]interface{})

		switch nodeOperation := nodeInnerMap["relOp"]; nodeOperation {
		case "LogicalTableScan":
			tableName := nodeInnerMap["table"].(string)
			rows = qe.tableScan(tableName)
		case "LogicalProject":
			selectedCols, colName = columnSelect(nodeInnerMap, referenceList, rows)
		case "LogicalFilter":
			filterByColumn(nodeInnerMap, referenceList, &rows)
		case "LogicalSort":
			sortAscDesc(nodeInnerMap, &rows)
		case "LogicalAggregate":
			groupBy(nodeInnerMap, colName, &rows, selectedCols)
		default:
			log.Fatalf("Unsupported Type: %s", nodeOperation)
		}
	}
}
