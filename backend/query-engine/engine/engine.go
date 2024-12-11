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
			qe.columnSelect(innerMap, rows)
		case "LogicalFilter":
			qe.filterByColumn(innerMap, plan, &rows)
		}
	}

}

func (qe *QueryEngine) filterByColumn(innerMap, plan map[string]interface{}, rows *[]*storage.RowV2) {
	conditionObj := innerMap["condition"].(map[string]interface{})
	operation := conditionObj["op"].(map[string]interface{})
	refList := plan["refList"].(map[string]interface{})

	switch kind := operation["kind"]; kind {
	case "GREATER_THAN", "LESS_THAN":
		intComparison(conditionObj["operands"], refList, rows, kind.(string))
	case "EQUALS":
		equals(conditionObj["operands"], refList, rows, kind.(string))
	default:
		log.Fatalf("kind %s not supported", kind)
	}
}

func equals(conditionObj interface{}, reflist map[string]interface{}, rows *[]*storage.RowV2, kind string) {
	maps := conditionObj.([]interface{})

	typeObj := maps[1].(map[string]interface{})
	typeMap := typeObj["type"].(map[string]interface{})
	typeName := typeMap["type"].(string)

	switch typeName {
	case "INTEGER", "BIGINT":
		intComparison(conditionObj, reflist, rows, kind)
	case "VARCHAR":
		charComparison(maps, reflist, rows)
	case "DECIMAL":
		decimalComparison(maps, reflist, rows)
	}

}

func decimalComparison(maps []interface{}, reflist map[string]interface{}, rows *[]*storage.RowV2) {
	var filteredRows []*storage.RowV2

	colNameObj := maps[0].(map[string]interface{})
	colNameMapSlice := colNameObj["operands"].([]interface{})
	colNameMap := colNameMapSlice[0].(map[string]interface{})
	colNameCode := colNameMap["name"].(string)

	colValObj := maps[1].(map[string]interface{})
	colValMapSlice := colValObj["operands"].([]interface{})
	colValMap := colValMapSlice[0].(map[string]interface{})
	operandsSlice := colValMap["operands"].([]interface{})
	operandMap := operandsSlice[0].(map[string]interface{})

	operandVal := operandMap["literal"].(string)
	colName := reflist[colNameCode].(string)

	for _, row := range *rows {
		fieldVal, ok := row.Values[colName]
		if !ok {
			log.Fatalf("field: %s not present in row: %d", colName, row.ID)
		}

		if fieldVal == operandVal {
			filteredRows = append(filteredRows, row)
		}
	}

	*rows = filteredRows
}

func charComparison(maps []interface{}, reflist map[string]interface{}, rows *[]*storage.RowV2) {
	var filteredRows []*storage.RowV2
	colNameMap := maps[0].(map[string]interface{})
	colNameCode := colNameMap["name"].(string)
	colName := reflist[colNameCode].(string)

	colComparisonMap := maps[1].(map[string]interface{})
	colComparisonVal := colComparisonMap["literal"].(string)

	for _, row := range *rows {
		fieldVal, ok := row.Values[colName]
		if !ok {
			log.Fatalf("field: %s not present in row: %d", colName, row.ID)
		}

		if fieldVal == colComparisonVal {
			filteredRows = append(filteredRows, row)
		}

	}

	*rows = filteredRows
}

func intComparison(conditionObj interface{}, reflist map[string]interface{}, rows *[]*storage.RowV2, kind string) {
	var filteredRows []*storage.RowV2
	maps := conditionObj.([]interface{})

	colObjMap := maps[0].(map[string]interface{})
	colNameMapSlice := colObjMap["operands"].([]interface{})
	colNameMap := colNameMapSlice[0].(map[string]interface{})

	valMap := maps[1].(map[string]interface{})

	colName := colNameMap["name"].(string)
	comparisonVal := int64(valMap["literal"].(float64))
	reference := reflist[colName].(string)

	for _, row := range *rows {
		fieldVal, ok := row.Values[reference]
		if !ok {
			log.Fatalf("field: %s not present in row: %d", reference, row.ID)
		}

		parsedUserVal, err := strconv.ParseInt(fieldVal, 10, 64)
		if err != nil {
			log.Fatalf("failed parsing %s, for row: %d", fieldVal, row.ID)
		}

		matchCondition := compare(parsedUserVal, comparisonVal, kind)
		if matchCondition {
			filteredRows = append(filteredRows, row)
		}
	}

	*rows = filteredRows
}

func compare(a, b int64, operator string) bool {
	switch operator {
	case "GREATER_THAN":
		return a > b
	case "LESS_THAN":
		return a < b
	case "EQUALS":
		return a == b
	default:
		return false
	}
}

func (qe *QueryEngine) columnSelect(nodeMap map[string]interface{}, rows []*storage.RowV2) {
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

			cleanedVal := strings.ReplaceAll(strRowVal, "'", "")
			newRow.Values[strRowCol] = cleanedVal
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
