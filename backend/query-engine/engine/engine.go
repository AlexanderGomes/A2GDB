package engine

import (
	"a2gdb/storage-engine/storage"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/scylladb/go-set/strset"
)

type QueryEngine struct {
	StorageManager *storage.DiskManagerV2
}

func (qe *QueryEngine) EngineEntry(queryPlan interface{}) {
	plan := queryPlan.(map[string]interface{})

	switch planOp := plan["BACKEND_OP"]; planOp {
	case "CREATE_TABLE":
		qe.handleCreate(plan)
	case "INSERT":
		qe.handleInsert(plan)
	case "SELECT":
		qe.handleSelect(plan)
	}
}

func (qe *QueryEngine) handleSelect(plan map[string]interface{}) {
	nodes := plan["rels"].([]interface{})
	var rows []*storage.RowV2

	for _, node := range nodes {
		innerMap := node.(map[string]interface{})
		switch op := innerMap["relOp"]; op {
		case "LogicalTableScan":
			rows = qe.tableScan(innerMap)
		case "LogicalProject":
			qe.filter(innerMap, rows)
		}
	}
}

func (qe *QueryEngine) filter(nodeMap map[string]interface{}, rows []*storage.RowV2) {
	columns := nodeMap["selected_columns"].([]interface{})
	set := strset.New()

	for _, column := range columns {
		columnStr := column.(string)
		cleanedColumn := strings.ReplaceAll(columnStr, "`", "")
		set.Add(cleanedColumn)
	}

	for _, row := range rows {
		for field := range row.Values {
			if !set.Has(field) {
				delete(row.Values, field)
			}
		}
	}
}

func (qe *QueryEngine) tableScan(nodeMap map[string]interface{}) []*storage.RowV2 {
	var rows []*storage.RowV2

	table := nodeMap["table"].([]interface{})
	tableName := table[0].(string)

	tableObj, err := qe.GetTable(tableName)
	if err != nil {
		log.Fatalf("GetTable failed for: %s, error: %s", tableName, err)
	}

	directoryMap := tableObj.DirectoryPage.Value
	pages, err := storage.GetTablePages(tableObj.DataFile, nil)
	if err != nil {
		log.Fatalf("GetTablePages failed for: %s, error: %s", tableName, err)
	}

	for _, page := range pages {
		pageId := storage.PageID(page.Header.ID)
		pageObj, ok := directoryMap[pageId]

		if !ok {
			log.Fatalf("PageObj not found for page: %v", page.Header.ID)
		}

		for _, location := range pageObj.PointerArray {
			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				log.Fatalf("DecodeRow error: %s", err)
			}

			rows = append(rows, row)
		}
	}

	return rows
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

func checkPresenceGetPrimary(selectedCols []interface{}, tableName string, catalog *storage.Catalog) (string, error) {
	var primary string

	// #check if table exist
	tableInfo, ok := catalog.Tables[storage.TableName(tableName)]
	if !ok {
		return "", fmt.Errorf("table: %s doesn't exist", tableName)
	}

	// #check if cols exist
	for _, selectedCol := range selectedCols {
		selectedCol := selectedCol.(string)

		_, ok := tableInfo.Schema[selectedCol]
		if !ok {
			return "", fmt.Errorf("column: %s on table: %s doesn't exist", selectedCol, tableName)
		}
	}

	//#get primary
	for column, columnInfo := range tableInfo.Schema {
		if columnInfo.IsIndex {
			primary = column
		}
	}

	return primary, nil
}

func prepareRows(plan map[string]interface{}, selectedCols []interface{}, tableName, primary string) (uint16, []uint64, [][]byte) {
	var bytesNeeded uint16
	rowsID := []uint64{}
	encodedRows := [][]byte{}

	interfaceRows := plan["rows"].([]interface{})

	for _, row := range interfaceRows {
		newRow := storage.RowV2{
			ID:     storage.GenerateRandomID(),
			Values: make(map[string]string),
		}

		//#Add row values
		newRow.Values[primary] = strconv.FormatUint(newRow.ID, 10)
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

		bytesNeeded += uint16(len(encodedRow))
		encodedRows = append(encodedRows, encodedRow)
		rowsID = append(rowsID, newRow.ID)
	}

	return bytesNeeded, rowsID, encodedRows
}

func findAndUpdate(tableObj *storage.TableObj, bytesNeeded uint16, tableName string, encodedRows [][]byte, rowsID []uint64) error {
	page, err := storage.FindAvailablePage(tableObj.DataFile, bytesNeeded, false)
	if err != nil {
		return fmt.Errorf("available page for table %s not found", tableName)
	}

	for _, encodedRow := range encodedRows {
		err := page.AddTuple(encodedRow)
		if err != nil {
			return fmt.Errorf("failed adding row %s, for table: %s, rrror: %s", encodedRow, tableName, err)
		}
	}

	err = updatePageInfo(rowsID, page, tableObj)
	if err != nil {
		return fmt.Errorf("tnternal update failed: %v", page)
	}

	return nil
}

func (qe *QueryEngine) GetTable(tableName string) (*storage.TableObj, error) {
	var tableObj *storage.TableObj
	var err error
	manager := qe.StorageManager

	tableObj, found := manager.TableObjs[storage.TableName(tableName)]
	if !found {
		tableObj, err = manager.InMemoryTableSetUp(storage.TableName(tableName))
		if err != nil {
			return nil, fmt.Errorf("GetTable: %w", err)
		}
	}

	return tableObj, err
}
