package engine

import (
	"a2gdb/logger"
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

type Result struct {
	Error   error
	Msg     string
	Rows    []*RowV2
	groupBy map[string]int
}

func (qe *QueryEngine) handleUpdate(plan map[string]interface{}, transactionOff bool, induceErr bool) Result {
	logger.Log.Info("Update Started")

	var result Result

	filterColumn := plan["filter_column"].(string)

	modifyColumn := plan["modify_column"].(string)
	modifyValue := plan["modify_value"].(string)

	tableName := plan["table"].(string)
	manager := qe.BufferPoolManager.DiskManager
	walManager := qe.BufferPoolManager.Wal

	tableObj, err := GetTableObj(tableName, manager)
	if err != nil {
		result.Error = fmt.Errorf("GetTableObj failed: %w", err)
		result.Msg = "failed"
		return result
	}

	tableStats := manager.PageCatalog.Tables[tableName]

	isPrimary, err := isPrimary(filterColumn, tableName, manager.PageCatalog)
	if err != nil {
		result.Error = fmt.Errorf("isPrimary failed: %w", err)
		result.Msg = "failed"
		return result
	}

	var filterValue string = strings.ReplaceAll(plan["filter_value"].(string), "'", "")
	if isPrimary {
		re := regexp.MustCompile(`\d+`)
		filterValue = re.FindString(filterValue)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	errChan := make(chan error, 4)
	pageChan := make(chan *PageV2, 100)
	updateInfoChan := make(chan ModifiedInfo, 100)
	insertChan := make(chan *NonAddedRows, 100)

	var txId string
	if !transactionOff {
		txId = walManager.BeginTransaction()
	}

	tasks := []func() error{
		func() error {
			return qe.BufferPoolManager.FullTableScan(ctx, pageChan, tableObj, tableStats.NumOfPages)
		},
		func() error {
			return processPagesForUpdate(ctx, pageChan, updateInfoChan, modifyColumn, modifyValue, filterColumn, filterValue, txId, tableObj, walManager, transactionOff)
		},
		func() error {
			return cleanOrgnize(ctx, updateInfoChan, insertChan, tableObj, tableStats)
		},
		func() error {
			return handleLikeInsert(ctx, insertChan, tableObj, tableName, qe.BufferPoolManager, tableStats)
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

	firstError := <-errChan
	if firstError != nil || induceErr {
		primary, primaryErr := getPrimary(tableName, manager.PageCatalog)
		if primaryErr != nil {
			result.Error = fmt.Errorf("couldn't get primary: %w", primaryErr)
			result.Msg = "failed"
			return result
		}

		return rollbackAndReturn(txId, primary, modifyColumn, walManager, qe, nil, fmt.Errorf("error occurred during update: %w", err), "failed")
	}

	if !transactionOff {
		if err := walManager.CommitTransaction(txId); err != nil {
			result.Error = fmt.Errorf("CommitTransaction failed: %w", err)
			result.Msg = "failed"
			return result
		}
	}

	result.Msg = "success"
	return result
}

func (qe *QueryEngine) handleDelete(plan map[string]interface{}, transactionOff, induceErr bool) Result {
	var result Result

	manager := qe.BufferPoolManager.DiskManager
	walManager := qe.BufferPoolManager.Wal
	catalog := qe.BufferPoolManager.DiskManager.PageCatalog

	tableName := plan["table"].(string)
	tableStats := manager.PageCatalog.Tables[tableName]

	deleteKey := plan["column"].(string)

	tableObj, err := GetTableObj(tableName, manager)
	if err != nil {
		result.Error = fmt.Errorf("GetTableObj failed: %w", err)
		result.Msg = "failed"
		return result
	}

	isPrimary, err := isPrimary(deleteKey, tableName, manager.PageCatalog)
	if err != nil {
		result.Error = fmt.Errorf("isPrimary failed: %w", err)
		result.Msg = "failed"
		return result
	}

	var deleteVal string = strings.ReplaceAll(plan["value"].(string), "'", "")
	if isPrimary {
		re := regexp.MustCompile(`\d+`)
		deleteVal = re.FindString(deleteVal)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	errChan := make(chan error, 3)

	pageChan := make(chan *PageV2, 100)
	updateInfoChan := make(chan ModifiedInfo, 100)

	var txId string
	if !transactionOff {
		txId = walManager.BeginTransaction()
	}

	tasks := []func() error{
		func() error {
			return qe.BufferPoolManager.FullTableScan(ctx, pageChan, tableObj, tableStats.NumOfPages)
		},
		func() error {
			return processPagesForDeletion(ctx, pageChan, updateInfoChan, deleteKey, deleteVal, txId, isPrimary, tableObj, walManager, transactionOff)
		},
		func() error {
			return cleanOrgnize(ctx, updateInfoChan, nil, tableObj, tableStats)
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

	firstError := <-errChan
	if firstError != nil || induceErr {
		primary, primaryErr := getPrimary(tableName, manager.PageCatalog)
		if primaryErr != nil {
			result.Error = fmt.Errorf("couldn't get primary: %w", primaryErr)
			result.Msg = "failed"
			return result
		}

		return rollbackAndReturn(txId, primary, "", walManager, qe, catalog, fmt.Errorf("error occurred during delete: %w", err), "failed")
	}

	if !transactionOff {
		if err := walManager.CommitTransaction(txId); err != nil {
			result.Error = fmt.Errorf("CommitTransaction failed: %w", err)
			result.Msg = "failed"
			return result
		}
	}

	result.Msg = "success"
	return result
}

func (qe *QueryEngine) handleCreate(plan map[string]interface{}) Result {
	var result Result

	tableName := plan["table"].(string)
	columnsInfo := plan["columns"].([]interface{})

	tableInfo := TableInfo{Schema: make(map[string]ColumnType)}

	for _, columnInfo := range columnsInfo {
		columnMap := columnInfo.(map[string]interface{})

		for colName, colType := range columnMap {
			cleanColName := strings.ReplaceAll(colName, "`", "")
			colTypeStr := colType.(string)

			tableInfo.Schema[cleanColName] = ColumnType{Type: colTypeStr, IsIndex: colTypeStr == "PRIMARY"}
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

func (qe *QueryEngine) handleInsert(plan map[string]interface{}, transactionOff bool) Result {
	logger.Log.Info("Insertion Started")

	manager := qe.BufferPoolManager.DiskManager
	walManager := qe.BufferPoolManager.Wal
	catalog := manager.PageCatalog

	selectedCols := plan["selectedCols"].([]interface{})
	tableName := plan["table"].(string)
	tableStats := catalog.Tables[tableName]

	primary, err := checkPresenceGetPrimary(selectedCols, tableName, catalog)
	if err != nil {
		return handleError(err, "failed")
	}

	tableobj, err := GetTableObj(tableName, manager)
	if err != nil {
		return handleError(fmt.Errorf("GetTable failed for: %s, error: %s", tableName, err), "failed")
	}

	var txId string
	if !transactionOff {
		txId = walManager.BeginTransaction()
	}

	bytesNeeded, encodedRows, err := prepareRows(plan, selectedCols, primary, tableName, txId, walManager, transactionOff)
	if err != nil {
		return rollbackAndReturn(txId, primary, "", walManager, qe, nil, fmt.Errorf("preparing rows failed: %w", err), "failed")
	}

	logger.Log.WithFields(logrus.Fields{
		"table":        tableName,
		"selectedCols": selectedCols,
		"primary":      primary,
		"bytesNeeded":  bytesNeeded,
	}).Info("findAndUpdate Inputs Set")

	err = findAndUpdate(qe.BufferPoolManager, tableobj, tableStats, bytesNeeded, tableName, encodedRows)
	if err != nil {
		return rollbackAndReturn(txId, primary, "", walManager, qe, nil, fmt.Errorf("findAndUpdate Failed: %s", err), "failed")
	}

	if !transactionOff {
		err = walManager.CommitTransaction(txId)
		if err != nil {
			return rollbackAndReturn(txId, primary, "", walManager, qe, nil, fmt.Errorf("CommitTransaction Failed: %s", err), "failed")
		}
	}

	return Result{Msg: "Tuples Inserted"}
}
