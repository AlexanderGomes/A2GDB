package engine

import (
	"a2gdb/logger"
	"a2gdb/storage-engine/storage"
	"log"
	"strings"

	"github.com/sirupsen/logrus"
)

func (qe *QueryEngine) handleUpdate(plan map[string]interface{}) {
	logger.Log.Info("Update Started")

	filterColumn := plan["filter_column"].(string)
	filterValue := strings.ReplaceAll(plan["filter_value"].(string), "'", "")

	modifyColumn := plan["modify_column"].(string)
	modifyValue := plan["modify_value"].(string)

	tableName := plan["table"].(string)
	manager := qe.BufferPoolManager.DiskManager

	tableObj, err := storage.GetTableObj(tableName, manager)
	if err != nil {
		log.Panicf("couldn't get table object for table %s, error: %s", tableName, err)
	}

	bufferPages := qe.BufferPoolManager.FullBufferScan()
	tablePages, err := storage.GetTablePagesFromDisk(tableObj.DataFile, nil, qe.BufferPoolManager.PageTable)
	if err != nil {
		log.Panicf("couldn't get table pages for table %s, error: %s", tableName, err)
	}

	var mergedPages []*storage.PageV2
	mergedPages = append(mergedPages, tablePages...)
	mergedPages = append(mergedPages, bufferPages...)

	logger.Log.WithFields(logrus.Fields{"filterColumn": filterColumn, "filterValue": filterValue, "modifyColumn": modifyColumn, "modifyValue": modifyValue, "tableName": tableName}).Info("processPagesForUpdate inputs")

	freeSpaceMapping, nonAddedRows := processPagesForUpdate(mergedPages, modifyColumn, modifyValue, filterColumn, filterValue, tableObj)

	var rowIds []uint64
	rowIds = append(rowIds, 0) // delete from bp
	for _, row := range nonAddedRows {
		rowIds = append(rowIds, row.Id)
	}

	cleanOrgnize(freeSpaceMapping, rowIds, tableObj, qe.BufferPoolManager)    // deletes old
	handleLikeInsert(nonAddedRows, tableObj, tableName, qe.BufferPoolManager) // inserts new

	logger.Log.Info("Update Completed")
}

func (qe *QueryEngine) handleDelete(plan map[string]interface{}) {
	logger.Log.Info("Delete Started")

	tableName := plan["table"].(string)
	deleteKey := plan["column"].(string)
	deleteVal := strings.ReplaceAll(plan["value"].(string), "'", "")

	manager := qe.BufferPoolManager.DiskManager

	tableObj, err := storage.GetTableObj(tableName, manager)
	if err != nil {
		log.Panicf("couldn't get table object for table %s, error: %s", tableName, err)
	}

	var mergedPages []*storage.PageV2

	bufferPages := qe.BufferPoolManager.FullBufferScan()
	tablePages, err := storage.GetTablePagesFromDisk(tableObj.DataFile, nil, qe.BufferPoolManager.PageTable)
	if err != nil {
		log.Panicf("couldn't get table pages for table %s, error: %s", tableName, err)
	}

	mergedPages = append(mergedPages, tablePages...)
	mergedPages = append(mergedPages, bufferPages...)

	logger.Log.WithFields(logrus.Fields{"deleteKey": deleteKey, "deleteVal": deleteVal, "tableName": tableName}).Info("processPagesForDeletion inputs")
	freeSpaceMapping, rowsID := processPagesForDeletion(mergedPages, deleteKey, deleteVal, tableObj)

	cleanOrgnize(freeSpaceMapping, rowsID, tableObj, qe.BufferPoolManager)
	logger.Log.Info("Delete Completed")
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

	err := qe.BufferPoolManager.DiskManager.CreateTable(storage.TableName(tableName), tableInfo)
	if err != nil {
		log.Fatal("Error Creating Table: ", err)
	}
}

func (qe *QueryEngine) handleInsert(plan map[string]interface{}) {
	logger.Log.Info("Insertion Started")

	manager := qe.BufferPoolManager.DiskManager
	catalog := manager.PageCatalog

	selectedCols := plan["selectedCols"].([]interface{})
	tableName := plan["table"].(string)

	primary, err := checkPresenceGetPrimary(selectedCols, tableName, catalog)
	if err != nil {
		log.Fatalf("checkPresence failed: %s", err)
	}

	bytesNeeded, rowsID, encodedRows := prepareRows(plan, selectedCols, tableName, primary)

	tableobj, err := storage.GetTableObj(tableName, manager)
	if err != nil {
		log.Fatalf("GetTable failed for: %s, error: %s", tableName, err)
	}

	logger.Log.WithFields(logrus.Fields{
		"table":        tableName,
		"selectedCols": selectedCols,
		"primary":      primary,
		"bytesNeeded":  bytesNeeded,
	}).Info("findAndUpdate Inputs Set")

	err = findAndUpdate(qe.BufferPoolManager, tableobj, bytesNeeded, tableName, encodedRows, rowsID)
	if err != nil {
		log.Fatalf("findAndUpdate Failed: %s", err)
	}
}
