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

	tableObj, err := qe.GetTableObj(tableName)
	if err != nil {
		log.Panicf("couldn't get table object for table %s, error: %s", tableName, err)
	}

	tablePages, err := storage.GetTablePages(tableObj.DataFile, nil)
	if err != nil {
		log.Panicf("couldn't get table pages for table %s, error: %s", tableName, err)
	}

	logger.Log.WithFields(logrus.Fields{"filterColumn": filterColumn, "filterValue": filterValue, "modifyColumn": modifyColumn, "modifyValue": modifyValue, "tableName": tableName}).Info("processPagesForUpdate inputs")

	freeSpaceMapping, nonAddedRows := processPagesForUpdate(tablePages, modifyColumn, modifyValue, filterColumn, filterValue, tableObj)

	cleanOrgnize(freeSpaceMapping, tableObj)            // deletes old
	handleLikeInsert(nonAddedRows, tableObj, tableName) // inserts new

	logger.Log.Info("Update Completed")
}

func (qe *QueryEngine) handleDelete(plan map[string]interface{}) {
	logger.Log.Info("Delete Started")
	tableName := plan["table"].(string)

	tableObj, err := qe.GetTableObj(tableName)
	if err != nil {
		log.Panicf("couldn't get table object for table %s, error: %s", tableName, err)
	}

	tablePages, err := storage.GetTablePages(tableObj.DataFile, nil)
	if err != nil {
		log.Panicf("couldn't get table pages for table %s, error: %s", tableName, err)
	}

	deleteKey := plan["column"].(string)
	deleteVal := strings.ReplaceAll(plan["value"].(string), "'", "")

	logger.Log.WithFields(logrus.Fields{"deleteKey": deleteKey, "deleteVal": deleteVal, "tableName": tableName}).Info("processPagesForDeletion inputs")
	freeSpaceMapping := processPagesForDeletion(tablePages, deleteKey, deleteVal, tableObj)

	cleanOrgnize(freeSpaceMapping, tableObj) // deletes old
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

	err := qe.StorageManager.CreateTable(storage.TableName(tableName), tableInfo)
	if err != nil {
		log.Fatal("Error Creating Table: ", err)
	}
}

func (qe *QueryEngine) handleInsert(plan map[string]interface{}) {
	logger.Log.Info("Insertion Started")

	manager := qe.StorageManager
	catalog := manager.PageCatalog

	selectedCols := plan["selectedCols"].([]interface{})
	tableName := plan["table"].(string)

	primary, err := checkPresenceGetPrimary(selectedCols, tableName, catalog)
	if err != nil {
		log.Fatalf("checkPresence failed: %s", err)
	}

	bytesNeeded, rowsID, encodedRows := prepareRows(plan, selectedCols, tableName, primary)

	tableobj, err := qe.GetTableObj(tableName)
	if err != nil {
		log.Fatalf("GetTable failed for: %s, error: %s", tableName, err)
	}

	logger.Log.WithFields(logrus.Fields{
		"table":        tableName,
		"selectedCols": selectedCols,
		"primary":      primary,
		"bytesNeeded":  bytesNeeded,
	}).Info("findAndUpdate Inputs Set")

	err = findAndUpdate(tableobj, bytesNeeded, tableName, encodedRows, rowsID)
	if err != nil {
		log.Fatalf("findAndUpdate Failed: %s", err)
	}
}
