package queryengine

import (
	"disk-db/storage"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
)

const MAX_FILE_SIZE = 1 * 1024 * 1024 * 1024 // 1 GB

type Query struct {
	Result  []*storage.RowV2
	Message string
}

type QueryEngine struct {
	DB *storage.BufferPoolManager
}

func (qe *QueryEngine) QueryEntryPoint(sql string) (Query, error) {
	parsedSQL, err := Parser(sql)
	if err != nil {
		return Query{}, fmt.Errorf("QueryEntryPoint: %w", err)
	}

	queryPlan := GenerateQueryPlan(parsedSQL)

	result, err := qe.ExecuteQueryPlan(queryPlan, parsedSQL)
	if err != nil {
		return Query{}, fmt.Errorf("QueryEntryPoint: %w", err)
	}

	return result, nil
}

func (qe *QueryEngine) ExecuteQueryPlan(qp ExecutionPlan, P *ParsedQuery) (Query, error) {
	var err error
	var offset storage.Offset

	query := Query{}
	tablesPtr := []*storage.TableObj{}
	tableObj := storage.TableObj{}

	for _, steps := range qp.Steps {
		if err != nil {
			return Query{}, fmt.Errorf("ExecuteQueryPlan: %w", err)
		}

		switch steps.Operation {
		case "GetTable":
			tableObj, err = GetTable(P, qe.DB, steps)
		case "GetAllColumns":
			err = GetAllColumns(&tableObj, &query, offset)
		case "CollectPointer":
			tablesPtr = append(tablesPtr, &tableObj)
		case "FilterByColumns":
			err = FilterByColumns(&tableObj, &query, P, offset)
		case "InsertRows":
			err = InsertRows(P, &query, qe.DB, &tableObj)
		case "CreateTable":
			err = CreateTable(P, &query, qe.DB)
		case "JoinQueryTable":
			err = JoinTables(&query, P.Joins[0].Condition, tablesPtr)
		case "DeleteFromTable":
			err = DeleteFromTable(P, qe.DB.DiskScheduler.DiskManager, &tableObj, offset)
		case "WhereClause":
			err = WhereClause(P, &query)
		case "Update":
			err = Update(P, qe.DB.DiskScheduler.DiskManager, &tableObj, offset)
		case "DetermineScan":
			offset, err = DetermineScan(P, qe.DB.DiskScheduler.DiskManager)
		}
	}

	return query, nil
}

func DetermineScan(p *ParsedQuery, dm *storage.DiskManagerV2) (storage.Offset, error) {
	var offset storage.Offset
	whereField := p.Where[0]
	whereValue := p.Where[1]
	uintValue, err := strconv.ParseUint(whereValue, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to convert string to uint64: %w", err)
	}

	tableName := storage.TableName(p.TableReferences[0])
	tableInfo := dm.PageCatalog.Tables[tableName]
	columnType := tableInfo.Schema[whereField]

	if columnType.IsIndex {
		tableObj := dm.TableObjs[tableName]
		item, err := storage.GetItemByKey(tableObj.BpTree, uintValue)
		if err != nil {
			return 0, fmt.Errorf("DetermineScan (wrong primary key): %w", err)
		}

		offset = item.Value
	}

	return offset, nil
}

func Update(p *ParsedQuery, manager *storage.DiskManagerV2, tableObj *storage.TableObj, offset storage.Offset) error {
	var tablePages []*storage.PageV2
	var err error

	tablePages, err = getTablePages(tableObj, offset)
	if err != nil {
		return fmt.Errorf("UPDATE: %w", err)
	}

	findField := p.Where[0]
	findValue := p.Where[1]

	changingField := p.Predicates[0].(string)
	newValue := p.Predicates[1].(string)

	if err := processPages(tablePages, findField, findValue, changingField, newValue); err != nil {
		return fmt.Errorf("UPDATE: %w", err)
	}

	if err := writeUpdatedPages(tablePages, manager, tableObj); err != nil {
		return fmt.Errorf("UPDATE: %w", err)
	}

	return nil
}

func getTablePages(tableObj *storage.TableObj, offset storage.Offset) ([]*storage.PageV2, error) {
	stat, _ := tableObj.DataFile.Stat()
	size := stat.Size()

	if offset == 0 {
		if size >= MAX_FILE_SIZE {
			return storage.FullTableScanBigFiles(tableObj.DataFile)
		}
		return storage.FullTableScan(tableObj.DataFile)
	}

	bytes, err := storage.ReadPageAtOffset(tableObj.DataFile, offset)
	if err != nil {
		return nil, fmt.Errorf("getTablePages: %w", err)
	}
	page, err := storage.DecodePageV2(bytes)
	if err != nil {
		return nil, fmt.Errorf("getTablePages: %w", err)
	}
	return []*storage.PageV2{page}, nil
}

func processPages(pages []*storage.PageV2, findField, findValue, changingField, newValue string) error {
	for _, page := range pages {
		for _, location := range page.PointerArray {
			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				return fmt.Errorf("processPages: %w", err)
			}

			if row.Values[findField] == findValue {
				row.Values[changingField] = newValue
				updatedBytes, err := storage.SerializeRow(row)
				if err != nil {
					return fmt.Errorf("processPages: %w", err)
				}
				page.AddTuple(updatedBytes)
			}
		}
	}
	return nil
}

func writeUpdatedPages(pages []*storage.PageV2, manager *storage.DiskManagerV2, tableObj *storage.TableObj) error {
	for _, page := range pages {
		pageObj := tableObj.DirectoryPage.Value[storage.PageID(page.Header.ID)]
		if err := manager.WritePageBackV2(page, pageObj.Offset, tableObj.DataFile); err != nil {
			return fmt.Errorf("writeUpdatedPages: %w", err)
		}
	}
	return nil
}

func WhereClause(p *ParsedQuery, q *Query) error {
	if len(p.Predicates) < 3 {
		return errors.New("WhereClause (insufficient predicates)")
	}

	field, ok := p.Predicates[0].(string)
	if !ok {
		return errors.New("WhereClause (first predicate is not a string)")
	}
	condition, ok := p.Predicates[1].(string)
	if !ok {
		return errors.New("WhereClause (second predicate is not a string)")
	}
	value, ok := p.Predicates[2].(string)
	if !ok {
		return errors.New("WhereClause (third predicate is not a string)")
	}

	if condition != "=" {
		return errors.New("WhereClause (unsupported condition)")
	}

	res := []*storage.RowV2{}
	for _, row := range q.Result {
		rowVal, ok := row.Values[field]
		if !ok {
			return fmt.Errorf("field %s not found in row", field)
		}
		if rowVal == value {
			res = append(res, row)
		}
	}

	q.Result = res
	return nil
}

func DeleteFromTable(p *ParsedQuery, manager *storage.DiskManagerV2, tableObj *storage.TableObj, offset storage.Offset) error {
	var tablePages []*storage.PageV2
	var err error

	tablePages, err = getTablePages(tableObj, offset)
	if err != nil {
		return fmt.Errorf("DELETE: %w", err)
	}

	field := p.Where[0]
	value := p.Where[1]

	if err := processPagesForDeletion(tablePages, field, value); err != nil {
		return fmt.Errorf("DELETE: %w", err)
	}

	if err := writeUpdatedPages(tablePages, manager, tableObj); err != nil {
		return fmt.Errorf("DELETE: %w", err)
	}

	return nil
}

func processPagesForDeletion(pages []*storage.PageV2, field, value string) error {
	for _, page := range pages {
		for _, location := range page.PointerArray {
			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				return fmt.Errorf("processPagesForDeletion: %w", err)
			}

			if row.Values[field] == value {
				storage.ResetBytesToEmpty(page, location.Offset, location.Length)
			}
		}
	}
	return nil
}

func JoinTables(query *Query, condition string, tablePtrs []*storage.TableObj) error {
	if len(tablePtrs) != 2 {
		return fmt.Errorf("JoinTables: expected exactly two tables")
	}

	slicePage1, err := storage.FullTableScan(tablePtrs[0].DataFile)
	if err != nil {
		return fmt.Errorf("JoinTables (error reading table one): %w", err)
	}

	slicePage2, err := storage.FullTableScan(tablePtrs[1].DataFile)
	if err != nil {
		return fmt.Errorf("JoinTables (error reading table two): %w", err)
	}

	leftTableCondition, rightTableCondition, err := parseJoinCondition(condition)
	if err != nil {
		return fmt.Errorf("JoinTables (error parsing condition): %w", err)
	}

	hashTable := buildHashTable(slicePage1, tablePtrs[0].DirectoryPage, leftTableCondition)
	if err := joinAndStoreResults(slicePage2, tablePtrs[1].DirectoryPage, rightTableCondition, hashTable, query); err != nil {
		return fmt.Errorf("JoinTables (error joining and storing results): %w", err)
	}

	return nil
}

func parseJoinCondition(condition string) (string, string, error) {
	parts := strings.Split(condition, "=")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid join condition format")
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

func buildHashTable(pages []*storage.PageV2, dirPage *storage.DirectoryPageV2, conditionField string) map[string]*storage.RowV2 {
	hashTable := make(map[string]*storage.RowV2)

	for _, page := range pages {
		pageObj := dirPage.Value[storage.PageID(page.Header.ID)]
		for _, location := range pageObj.PointerArray {
			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				continue
			}
			joinKey := row.Values[conditionField]
			hashTable[joinKey] = row
		}
	}
	return hashTable
}

func joinAndStoreResults(pages []*storage.PageV2, dirPage *storage.DirectoryPageV2, conditionField string, hashTable map[string]*storage.RowV2, query *Query) error {
	for _, page := range pages {
		pageObj := dirPage.Value[storage.PageID(page.Header.ID)]
		for _, location := range pageObj.PointerArray {
			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				return fmt.Errorf("joinAndStoreResults: %w", err)
			}
			joinKey := row.Values[conditionField]
			if matchedRow, exists := hashTable[joinKey]; exists {
				query.Result = append(query.Result, matchedRow)
			}
		}
	}
	return nil
}

func CreateTable(parsedQuery *ParsedQuery, query *Query, bpm *storage.BufferPoolManager) error {
	tableName := parsedQuery.TableReferences[0]
	manager := bpm.DiskScheduler.DiskManager

	tableSchema, err := buildTableSchema(parsedQuery)
	if err != nil {
		return fmt.Errorf("CreateTable (error building schema): %w", err)
	}

	if err := manager.CreateTable(storage.TableName(tableName), tableSchema); err != nil {
		return fmt.Errorf("CreateTable (error creating table): %w", err)
	}

	log.Println("TABLE CREATED")
	return nil
}

func buildTableSchema(parsedQuery *ParsedQuery) (storage.TableInfo, error) {
	tableInfo := storage.TableInfo{Schema: make(map[string]storage.ColumnType)}

	for i, columnName := range parsedQuery.ColumnsSelected {
		columnInfo, ok := parsedQuery.Predicates[i].(storage.ColumnType)
		if !ok {
			return tableInfo, fmt.Errorf("invalid column type for column: %s", columnName)
		}

		if columnInfo.IsIndex {
			columnInfo.Type = "INT64"
		}

		tableInfo.Schema[columnName] = columnInfo
	}

	return tableInfo, nil
}

func GetTable(parsedQuery *ParsedQuery, bpm *storage.BufferPoolManager, step QueryStep) (storage.TableObj, error) {
	log.Println("GETTING TABLE")

	manager := bpm.DiskScheduler.DiskManager
	tableNAME := parsedQuery.TableReferences[step.index]

	var tableObj *storage.TableObj
	var err error
	
	tableObj, found := manager.TableObjs[storage.TableName(tableNAME)]
	if !found {
		tableObj, err = manager.InMemoryTableSetUp(storage.TableName(tableNAME))
		if err != nil {
			return storage.TableObj{}, fmt.Errorf("GetTable: %w", err)
		}
	}

	return *tableObj, err
}

func InsertRows(parsedQuery *ParsedQuery, query *Query, bpm *storage.BufferPoolManager, tableObj *storage.TableObj) error {
	log.Println("INSERTING ROWS")

	encodedRows, spaceNeeded, err := serializeRows(parsedQuery.Predicates)
	if err != nil {
		return fmt.Errorf("InsertRows: %w", err)
	}

	pageFound, err := storage.FindAvailablePage(tableObj.DataFile, spaceNeeded)
	if err != nil {
		return fmt.Errorf("InsertRows: %w", err)
	}

	if err := addRowsToPage(pageFound, encodedRows); err != nil {
		return fmt.Errorf("InsertRows: %w", err)
	}

	pageID := storage.PageID(pageFound.Header.ID)
	if err := updatePageInfo(pageID, pageFound, tableObj, bpm, parsedQuery); err != nil {
		return fmt.Errorf("InsertRows: %w", err)
	}

	return nil
}

func serializeRows(rows []interface{}) ([][]byte, int, error) {
	var encodedRows [][]byte
	var spaceNeeded int

	for _, row := range rows {
		rowV2, ok := row.(*storage.RowV2)
		if !ok {
			return nil, 0, fmt.Errorf("row type assertion failed")
		}

		rowV2.ID = storage.GenerateRandomID()
		rowBytes, err := storage.SerializeRow(rowV2)
		if err != nil {
			return nil, 0, fmt.Errorf("serialization error: %w", err)
		}

		encodedRows = append(encodedRows, rowBytes)
		spaceNeeded += len(rowBytes)
	}

	return encodedRows, spaceNeeded, nil
}

func addRowsToPage(page *storage.PageV2, rows [][]byte) error {
	for _, rowBytes := range rows {
		if err := page.AddTuple(rowBytes); err != nil {
			return fmt.Errorf("add tuple error: %w", err)
		}
	}
	return nil
}

func updatePageInfo(pageID storage.PageID, pageFound *storage.PageV2, tableObj *storage.TableObj, bpm *storage.BufferPoolManager, pq *ParsedQuery) error {
	manager := bpm.DiskScheduler.DiskManager
	dirPage := tableObj.DirectoryPage
	pageInfObj, found := dirPage.Value[pageID]


	if !found {
		offset, err := manager.WritePageEOFV2(pageFound, tableObj.DataFile)
		if err != nil {
			return fmt.Errorf("write page EOF error: %w", err)
		}

		pageInfObj = &storage.PageInfo{
			Offset:       offset,
			PointerArray: pageFound.PointerArray,
		}

		dirPage.Value[pageID] = pageInfObj
		if err := manager.UpdateDirectoryPageDisk(dirPage, tableObj.DirFile); err != nil {
			return fmt.Errorf("update directory page error: %w", err)
		}
	} else {
		pageInfObj.PointerArray = append(pageInfObj.PointerArray, pageFound.PointerArray...)
		if err := manager.UpdateDirectoryPageDisk(dirPage, tableObj.DirFile); err != nil {
			return fmt.Errorf("update directory page error: %w", err)
		}

		if err := manager.WritePageBackV2(pageFound, pageInfObj.Offset, tableObj.DataFile); err != nil {
			return fmt.Errorf("write page back error: %w", err)
		}
	}

	if err := storage.UpdateBp(pq.Predicates, *tableObj, *pageInfObj); err != nil {
		return fmt.Errorf("update B+ tree error: %w", err)
	}

	return nil
}

func createColumnMap(columns []string) map[string]string {
	columnMap := make(map[string]string)

	for _, name := range columns {
		columnMap[name] = name
	}

	return columnMap
}

func FilterByColumns(tableObj *storage.TableObj, query *Query, P *ParsedQuery, offset storage.Offset) error {
	pageSlice, err := getTablePages(tableObj, offset)
	if err != nil {
		return fmt.Errorf("GetAllColumns: %w", err)
	}

	columnMap := createColumnMap(P.ColumnsSelected)
	dirPage := tableObj.DirectoryPage
	dirPageValues := dirPage.Value

	for _, page := range pageSlice {
		pageID := storage.PageID(page.Header.ID)
		pageObj, exists := dirPageValues[pageID]
		if !exists {
			return fmt.Errorf("FilterByColumns: page info not found for page ID %d", pageID)
		}

		for _, location := range pageObj.PointerArray {
			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				return fmt.Errorf("FilterByColumns: failed to decode row: %w", err)
			}

			tempTuple := storage.RowV2{Values: make(map[string]string)}
			for col := range columnMap {
				if value, found := row.Values[col]; found {
					tempTuple.Values[col] = value
				}
			}

			query.Result = append(query.Result, &tempTuple)
		}
	}

	return nil
}

func GetAllColumns(tableObj *storage.TableObj, query *Query, offset storage.Offset) error {
	pageSlice, err := getTablePages(tableObj, offset)
	if err != nil {
		return fmt.Errorf("GetAllColumns: %w", err)
	}

	dirPage := tableObj.DirectoryPage
	dirPageValues := dirPage.Value

	for _, page := range pageSlice {
		pageID := storage.PageID(page.Header.ID)
		pageObj, exists := dirPageValues[pageID]
		if !exists {
			return fmt.Errorf("GetAllColumns: page info not found for page ID %d", pageID)
		}

		for _, location := range pageObj.PointerArray {
			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				return fmt.Errorf("GetAllColumns: failed to decode row: %w", err)
			}

			query.Result = append(query.Result, row)
		}
	}

	return nil
}