package engine

import (
	"a2gdb/storage-engine/storage"
	"log"
	"strings"
)

// ## update && memory system
// - fits on the current page
//     - chek if it fits first, if it fails move on.
//     - adding to a new one changes their rank of memory
//     - removing from the current page changes their rank of memory

// - needs a new page
//     - remove from current page
//     - handle like insert into new page
//     - removing from the current page changes their rank of memory
//     - adding to a new one changes their rank of memory

func (qe *QueryEngine) handleUpdate(plan map[string]interface{}) {
	filterColumn := plan["filter_column"].(string)
	filterValue := strings.ReplaceAll(plan["filter_value"].(string), "'", "")

	modifyColumn := plan["modify_column"].(string)
	modifyValue := plan["modify_value"].(string)

	tableName := plan["table"].(string)

	tableObj, err := qe.GetTable(tableName)
	if err != nil {
		log.Panicf("couldn't get table object for table %s, error: %s", tableName, err)
	}

	tablePages, err := storage.GetTablePages(tableObj.DataFile, nil)
	if err != nil {
		log.Panicf("couldn't get table pages for table %s, error: %s", tableName, err)
	}

	freeSpaceMapping := processPagesForUpdate(tablePages, modifyColumn, modifyValue, filterColumn, filterValue, tableObj)

	for _, page := range tablePages {
		err := updatePageInfo(nil, page, tableObj)
		if err != nil {
			log.Panicf("writing to disk failed, table %s, page %+v", tableName, page)
		}
	}

	cleanOrgnize(freeSpaceMapping, tableObj)
}

func (qe *QueryEngine) handleDelete(plan map[string]interface{}) {
	tableName := plan["table"].(string)

	tableObj, err := qe.GetTable(tableName)
	if err != nil {
		log.Panicf("couldn't get table object for table %s, error: %s", tableName, err)
	}

	tablePages, err := storage.GetTablePages(tableObj.DataFile, nil)
	if err != nil {
		log.Panicf("couldn't get table pages for table %s, error: %s", tableName, err)
	}

	deleteKey := plan["column"].(string)
	deleteValStr := plan["value"].(string)
	cleanedVal := strings.ReplaceAll(deleteValStr, "'", "")

	freeSpaceMapping := processPagesForDeletion(tablePages, deleteKey, cleanedVal, tableObj)

	for _, page := range tablePages {
		err := updatePageInfo(nil, page, tableObj)
		if err != nil {
			log.Panicf("writing to disk failed, table %s, page %+v", tableName, page)
		}
	}

	cleanOrgnize(freeSpaceMapping, tableObj)
}

func (qe *QueryEngine) handleCreate(plan map[string]interface{}) {
	tableName := plan["table"].(string)
	columnsInfo := plan["columns"].([]interface{})

	tableInfo := storage.TableInfo{Schema: make(map[string]storage.ColumnType)}

	for _, columnInfo := range columnsInfo {
		columnMap := columnInfo.(map[string]interface{})

		for colName, colType := range columnMap {
			cleanColName := strings.ReplaceAll(colName, "`", "")
			colTypeStr := colType.(string)

			tableInfo.Schema[cleanColName] = storage.ColumnType{Type: colTypeStr, IsIndex: colTypeStr == "PRIMARY"}
		}
	}

	err := qe.StorageManager.CreateTable(storage.TableName(tableName), tableInfo)
	if err != nil {
		log.Println("Error Creating Table: ", err)
		return
	}
}

func (qe *QueryEngine) handleInsert(plan map[string]interface{}) {
	manager := qe.StorageManager

	catalog := manager.PageCatalog
	selectedCols := plan["selectedCols"].([]interface{})
	tableName := plan["table"].(string)

	primary, err := checkPresenceGetPrimary(selectedCols, tableName, catalog)
	if err != nil {
		log.Fatalf("checkPresence failed: %s", err)
	}

	bytesNeeded, rowsID, encodedRows := prepareRows(plan, selectedCols, tableName, primary)

	tableobj, err := qe.GetTable(tableName)
	if err != nil {
		log.Fatalf("GetTable failed for: %s, error: %s", tableName, err)
	}

	err = findAndUpdate(tableobj, bytesNeeded, tableName, encodedRows, rowsID)
	if err != nil {
		log.Fatalf("findAndUpdate Failed: %s", err)
	}
}
