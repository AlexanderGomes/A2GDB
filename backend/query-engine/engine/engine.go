package engine

import (
	"a2gdb/storage-engine/storage"
	"fmt"
)

type QueryEngine struct {
	StorageManager *storage.DiskManagerV2
}

func (qe *QueryEngine) EngineEntry(queryPlan interface{}) {
	kvJson := queryPlan.(map[string]interface{})

	switch planOp := kvJson["relOp"]; planOp {
	case "CREATE_TABLE":
		qe.handleCreate(kvJson)
	case "INSERT":

	case "SELECT":

	}
}

func (qe *QueryEngine) handleCreate(plan map[string]interface{}) {
	table := plan["tableName"]
	columns := plan["columns"]

	fmt.Println(table)
	fmt.Println(columns)
}
