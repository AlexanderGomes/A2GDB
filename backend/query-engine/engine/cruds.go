package engine

import (
	"a2gdb/logger"
	"a2gdb/storage-engine/storage"
	"context"
	"fmt"
	"strings"
	"sync"

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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	errChan := make(chan error, 4)
	pageChan := make(chan *storage.PageV2)
	updateInfoChan := make(chan ModifiedInfo)
	insertChan := make(chan *NonAddedRows)

	tasks := []func() error{
		func() error {
			return qe.BufferPoolManager.FullTableScan(ctx, pageChan, tableObj.DataFile, qe.BufferPoolManager.PageTable)
		},
		func() error {
			return processPagesForUpdate(ctx, pageChan, updateInfoChan, modifyColumn, modifyValue, filterColumn, filterValue, tableObj)
		},
		func() error {
			return cleanOrgnize(ctx, updateInfoChan, insertChan, tableObj, qe.BufferPoolManager)
		},
		func() error {
			return handleLikeInsert(ctx, insertChan, tableObj, tableName, qe.BufferPoolManager)
		},
	}

	for _, task := range tasks {
		wg.Add(1)
		go func(task func() error) {
			defer wg.Done()
			if err := task(); err != nil {
				errChan <- err
				cancel()
			}
		}(task)
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	for err := range errChan {
		if err != nil {
			result.Error = fmt.Errorf("error occurred during update: %w", err)
			result.Msg = "failed"
			return result
		}
	}

	result.Msg = "success"
	return result
}

func (qe *QueryEngine) handleDelete(plan map[string]interface{}) Result {
	var result Result

	// logger.Log.Info("Delete Started")

	// tableName := plan["table"].(string)
	// deleteKey := plan["column"].(string)
	// deleteVal := strings.ReplaceAll(plan["value"].(string), "'", "")

	// manager := qe.BufferPoolManager.DiskManager

	// tableObj, err := storage.GetTableObj(tableName, manager)
	// if err != nil {
	// 	result.Error = fmt.Errorf("GetTableObj failed: %w", err)
	// 	result.Msg = "failed"
	// 	return result
	// }

	// pageChan := make(chan *storage.PageV2)
	// pages, err := qe.BufferPoolManager.FullTableScan(pageChan, tableObj.DataFile, qe.BufferPoolManager.PageTable)
	// if err != nil {
	// 	result.Error = fmt.Errorf("FullTableScan failed: %w", err)
	// 	result.Msg = "failed"
	// 	return result
	// }

	// logger.Log.WithFields(logrus.Fields{"deleteKey": deleteKey, "deleteVal": deleteVal, "tableName": tableName}).Info("processPagesForDeletion inputs")
	// freeSpaceMapping, rowsID, err := processPagesForDeletion(pages, deleteKey, deleteVal, tableObj)
	// if err != nil {
	// 	result.Error = fmt.Errorf("processPagesForDeletion failed: %w", err)
	// 	result.Msg = "failed"
	// 	return result
	// }

	// err = cleanOrgnize(freeSpaceMapping, rowsID, tableObj, qe.BufferPoolManager)
	// if err != nil {
	// 	result.Error = fmt.Errorf("cleanOrgnize failed: %w", err)
	// 	result.Msg = "failed"
	// 	return result
	// }

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
