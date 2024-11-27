package engine

import (
	"a2gdb/storage-engine/storage"
	"fmt"
	"log"
	"strings"
)

type QueryEngine struct {
	StorageManager *storage.DiskManagerV2
}

func (qe *QueryEngine) EngineEntry(queryPlan interface{}) {
	plan := queryPlan.(map[string]interface{})

	switch planOp := plan["relOp"]; planOp {
	case "CREATE_TABLE":
		qe.handleCreate(plan)
	case "INSERT":
		qe.handleInsert(plan)
	case "SELECT":

	}
}

func (qe *QueryEngine) handleInsert(plan map[string]interface{}) {
	manager := qe.StorageManager
	//#check if table exist
	catalog := manager.PageCatalog.Tables
	tableName := plan["table"].(string)

	tableInfo, ok := catalog[storage.TableName(tableName)]
	if !ok {
		log.Printf("Table: %s doesn't exist", tableName)
		return
	}

	//#check if cols exist
	selectedCols := plan["selectedCols"].([]interface{})
	for _, selectedCol := range selectedCols {
		selectedCol := selectedCol.(string)

		_, ok := tableInfo.Schema[selectedCol]
		if !ok {
			log.Printf("Column: %s on Table: %s doesn't exist", selectedCol, tableName)
			return
		}
	}

	// ##Set up in memory data structures and encode the rows
	tableobj, err := manager.InMemoryTableSetUp(storage.TableName(tableName))
	if err != nil {
		log.Printf("Table: %s doesn't exist", tableName)
	}

	var bytesNeeded uint16
	rowsID := []uint64{}
	encodedRows := [][]byte{}

	interfaceRows := plan["rows"].([]interface{})

	for _, row := range interfaceRows {
		newRow := storage.RowV2{
			ID:     storage.GenerateRandomID(),
			Values: make(map[string]string),
		}

		fmt.Println(newRow.ID)

		//#Add rows values
		for i, rowVal := range row.([]interface{}) {
			strRowVal := rowVal.(string)
			strRowCol := selectedCols[i].(string)

			newRow.Values[strRowCol] = strRowVal
		}

		//#Encode rows
		encodedRow, err := storage.SerializeRow(&newRow)
		if err != nil {
			log.Printf("Failed Encoding row %v For Table: %s", row, tableName)
		}

		encodedRows = append(encodedRows, encodedRow)
		rowsID = append(rowsID, newRow.ID)
	}

	//#Update page and the necessary files
	page, err := storage.FindAvailablePage(tableobj.DataFile, bytesNeeded, false)
	if err != nil {
		log.Printf("Finding Available Page For Table: %s => NOT FOUND", tableName)
	}

	for i, encodedRow := range encodedRows {
		err := page.AddTuple(encodedRow)
		if err != nil {
			log.Printf("Failed Adding Row %s, at Index %d, for Table: %s, Error: %s", encodedRow, i, tableName, err)
			return
		}
	}

	err = updatePageInfo(rowsID, page, tableobj)
	if err != nil {
		log.Printf("Failed Update Internal State For Page: %v", page)
		return
	}
}

func (qe *QueryEngine) handleCreate(plan map[string]interface{}) {
	tableName := plan["table"].(string)
	columnsInfo := plan["columns"].([]interface{})

	tableInfo := storage.TableInfo{Schema: make(map[string]storage.ColumnType)}

	for _, columnInfo := range columnsInfo {
		columnMap := columnInfo.(map[string]interface{})

		for colName, colType := range columnMap {
			cleanColName := strings.ReplaceAll(colName, "`", "")
			tableInfo.Schema[cleanColName] = storage.ColumnType{Type: colType.(string), IsIndex: colType == "PRIMARY"}
		}
	}

	err := qe.StorageManager.CreateTable(storage.TableName(tableName), tableInfo)
	if err != nil {
		log.Println("Error Creating Table: ", err)
		return
	}
}

func updatePageInfo(rowsID []uint64, pageFound *storage.PageV2, tableObj *storage.TableObj) error {
	pageID := storage.PageID(pageFound.Header.ID)
	dirPage := tableObj.DirectoryPage
	pageInfObj, found := dirPage.Value[pageID]

	if !found {
		offset, err := storage.WritePageEOFV2(pageFound, tableObj.DataFile)
		if err != nil {
			return fmt.Errorf("write page EOF error: %w", err)
		}

		pageInfObj = &storage.PageInfo{
			Offset:       offset,
			PointerArray: pageFound.PointerArray,
		}

		dirPage.Value[pageID] = pageInfObj
	} else {
		pageInfObj.PointerArray = append(pageInfObj.PointerArray, pageFound.PointerArray...)
		if err := storage.WritePageBackV2(pageFound, pageInfObj.Offset, tableObj.DataFile); err != nil {
			return fmt.Errorf("write page back error: %w", err)
		}
	}

	if err := storage.UpdateDirectoryPageDisk(dirPage, tableObj.DirFile); err != nil {
		return fmt.Errorf("update directory page error: %w", err)
	}

	if err := storage.UpdateBp(rowsID, *tableObj, *pageInfObj); err != nil {
		return fmt.Errorf("update B+ tree error: %w", err)
	}

	return nil
}
