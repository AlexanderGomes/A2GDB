package engine

import (
	"a2gdb/logger"
	"a2gdb/storage-engine/storage"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
)

type Result struct {
	Error   error
	Msg     string
	Rows    []*storage.RowV2
	groupBy map[string]int
}

func (qe *QueryEngine) handleUpdate(plan map[string]interface{}) Result {
	logger.Log.Info("Update Started")

	var result Result

	filterColumn := plan["filter_column"].(string)
	filterValue := strings.ReplaceAll(plan["filter_value"].(string), "'", "")

	modifyColumn := plan["modify_column"].(string)
	modifyValue := plan["modify_value"].(string)

	tableName := plan["table"].(string)
	manager := qe.BufferPoolManager.DiskManager

	tableObj, err := storage.GetTableObj(tableName, manager)
	if err != nil {
		result.Error = fmt.Errorf("GetTableObj failed: %w", err)
		result.Msg = "failed"
		return result
	}

	pages, err := qe.BufferPoolManager.FullTableScan(tableObj.DataFile, qe.BufferPoolManager.PageTable)
	if err != nil {
		result.Error = fmt.Errorf("FullTableScan failed: %w", err)
		result.Msg = "failed"
		return result
	}

	logger.Log.WithFields(logrus.Fields{"filterColumn": filterColumn, "filterValue": filterValue, "modifyColumn": modifyColumn, "modifyValue": modifyValue, "tableName": tableName}).Info("processPagesForUpdate inputs")

	freeSpaceMapping, nonAddedRows, err := processPagesForUpdate(pages, modifyColumn, modifyValue, filterColumn, filterValue, tableObj)
	if err != nil {
		result.Error = fmt.Errorf("processPagesForUpdate failed: %w", err)
		result.Msg = "failed"
		return result
	}

	var rowIds []uint64
	rowIds = append(rowIds, 0) // delete from bp
	for _, row := range nonAddedRows {
		rowIds = append(rowIds, row.Id)
	}

	err = cleanOrgnize(freeSpaceMapping, rowIds, tableObj, qe.BufferPoolManager) // deletes old
	if err != nil {
		result.Error = fmt.Errorf("cleanOrgnize failed: %w", err)
		result.Msg = "failed"
		return result
	}

	err = handleLikeInsert(nonAddedRows, tableObj, tableName, qe.BufferPoolManager) // inserts new
	if err != nil {
		result.Error = fmt.Errorf("handleLikeInsert failed: %w", err)
		result.Msg = "failed"
		return result
	}

	logger.Log.Info("Update Completed")
	result.Msg = "Success"
	return result
}

func (qe *QueryEngine) handleDelete(plan map[string]interface{}) Result {
	var result Result

	logger.Log.Info("Delete Started")

	tableName := plan["table"].(string)
	deleteKey := plan["column"].(string)
	deleteVal := strings.ReplaceAll(plan["value"].(string), "'", "")

	manager := qe.BufferPoolManager.DiskManager

	tableObj, err := storage.GetTableObj(tableName, manager)
	if err != nil {
		result.Error = fmt.Errorf("GetTableObj failed: %w", err)
		result.Msg = "failed"
		return result
	}

	pages, err := qe.BufferPoolManager.FullTableScan(tableObj.DataFile, qe.BufferPoolManager.PageTable)
	if err != nil {
		result.Error = fmt.Errorf("FullTableScan failed: %w", err)
		result.Msg = "failed"
		return result
	}

	logger.Log.WithFields(logrus.Fields{"deleteKey": deleteKey, "deleteVal": deleteVal, "tableName": tableName}).Info("processPagesForDeletion inputs")
	freeSpaceMapping, rowsID, err := processPagesForDeletion(pages, deleteKey, deleteVal, tableObj)
	if err != nil {
		result.Error = fmt.Errorf("processPagesForDeletion failed: %w", err)
		result.Msg = "failed"
		return result
	}

	err = cleanOrgnize(freeSpaceMapping, rowsID, tableObj, qe.BufferPoolManager)
	if err != nil {
		result.Error = fmt.Errorf("cleanOrgnize failed: %w", err)
		result.Msg = "failed"
		return result
	}

	logger.Log.Info("Delete Completed")
	result.Msg = "Success"
	return result
}

func (qe *QueryEngine) handleCreate(plan map[string]interface{}) Result {
	var result Result

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

	err := qe.BufferPoolManager.DiskManager.CreateTable(tableName, tableInfo)
	if err != nil {
		result.Error = fmt.Errorf("CreateTable failed: %w", err)
		result.Msg = "failed"

		return result
	}

	result.Msg = "Table Created"

	return result
}

func (qe *QueryEngine) handleInsert(plan map[string]interface{}) Result {
	logger.Log.Info("Insertion Started")

	var result Result

	manager := qe.BufferPoolManager.DiskManager
	catalog := manager.PageCatalog

	selectedCols := plan["selectedCols"].([]interface{})
	tableName := plan["table"].(string)

	primary, err := checkPresenceGetPrimary(selectedCols, tableName, catalog)
	if err != nil {
		result.Error = err
		result.Msg = "failed"
		return result
	}

	tableobj, err := storage.GetTableObj(tableName, manager)
	if err != nil {
		result.Error = fmt.Errorf("GetTable failed for: %s, error: %s", tableName, err)
		result.Msg = "failed"
		return result
	}

	bytesNeeded, rowsID, encodedRows, err := prepareRows(plan, selectedCols, primary)
	if err != nil {
		result.Error = fmt.Errorf("preparing rows failed: %w", err)
		result.Msg = "failed"
		return result
	}

	logger.Log.WithFields(logrus.Fields{
		"table":        tableName,
		"selectedCols": selectedCols,
		"primary":      primary,
		"bytesNeeded":  bytesNeeded,
	}).Info("findAndUpdate Inputs Set")

	err = findAndUpdate(qe.BufferPoolManager, tableobj, bytesNeeded, tableName, encodedRows, rowsID)
	if err != nil {
		result.Error = fmt.Errorf("findAndUpdate Failed: %s", err)
		result.Msg = "failed"
		return result
	}

	result.Msg = "Tuples Inserted"

	return result
}
