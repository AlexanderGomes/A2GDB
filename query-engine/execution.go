package queryengine

import (
	"disk-db/storage"
	"errors"
	"fmt"
	"log"
	"strings"
)

type Query struct {
	Result  []*storage.RowV2
	Message string
}

type QueryEngine struct {
	DB *storage.BufferPoolManager
}

const (
	MAX_ROW_SIZE_BYTES = 150
)

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
			err = GetAllColumns(&tableObj, &query)
		case "CollectPointer":
			tablesPtr = append(tablesPtr, &tableObj)
		case "FilterByColumns":
			err = FilterByColumns(&tableObj, &query, P)
		case "InsertRows":
			err = InsertRows(P, &query, qe.DB, &tableObj)
		case "CreateTable":
			err = CreateTable(P, &query, qe.DB)
		case "JoinQueryTable":
			err = JoinTables(&query, P.Joins[0].Condition, tablesPtr)
		case "DeleteFromTable":
			err = DeleteFromTable(P, qe.DB.DiskScheduler.DiskManager, &tableObj)
		case "WhereClause":
			err = WhereClause(P, &query)
		}
	}

	return query, err
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

func DeleteFromTable(p *ParsedQuery, manager *storage.DiskManagerV2, tableObj *storage.TableObj) error {
	tablePages, err := storage.FullTableScan(tableObj.DataFile)
	if err != nil {
		return fmt.Errorf("DeleteFromTable: %w", err)
	}

	predicateStr := p.Predicates[0].(string)
	comparisonParts := strings.Split(predicateStr, "=")
	field := strings.TrimSpace(comparisonParts[0])
	value := strings.TrimSpace(comparisonParts[1])

	for _, page := range tablePages {
		dirPage := tableObj.DirectoryPage
		pageObj := dirPage.Value[storage.PageID(page.Header.ID)]
		tuplesInfo := pageObj.PointerArray

		for _, location := range tuplesInfo {

			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				return fmt.Errorf("DeleteFromTable: %w", err)
			}

			foundRow := row.Values[field] == value
			if foundRow {
				storage.ResetBytesToEmpty(page, location.Offset, location.Length)
			}
		}

		err := manager.WritePageBackV2(page, pageObj.Offset, tableObj.DataFile)
		if err != nil {
			return fmt.Errorf("DeleteFromTable: %w", err)
		}
	}

	return nil
}

func JoinTables(query *Query, condition string, tablePtr []*storage.TableObj) error {
	var err error
	var slicePage1, slicePage2 []*storage.PageV2

	slicePage1, err = storage.FullTableScan(tablePtr[0].DataFile)
	if err != nil {
		return fmt.Errorf("JoinTables (error reading table one): %w ", err)
	}

	slicePage2, err = storage.FullTableScan(tablePtr[1].DataFile)
	if err != nil {
		return fmt.Errorf("JoinTables (error reading table two): %w ", err)
	}

	comparisonParts := strings.Split(condition, "=")
	leftTableCondition := strings.TrimSpace(comparisonParts[0])
	rightTableCondition := strings.TrimSpace(comparisonParts[1])

	hashTable := make(map[string]*storage.RowV2)

	for _, page := range slicePage1 {
		dirPage := tablePtr[0].DirectoryPage
		pageObj := dirPage.Value[storage.PageID(page.Header.ID)]
		tuplesInfo := pageObj.PointerArray

		for _, location := range tuplesInfo {
			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				return fmt.Errorf("DeleteFromTable: %w", err)
			}

			joinKey := row.Values[leftTableCondition]
			hashTable[joinKey] = row
		}
	}

	for _, page := range slicePage2 {
		dirPage := tablePtr[1].DirectoryPage
		pageObj := dirPage.Value[storage.PageID(page.Header.ID)]
		tuplesInfo := pageObj.PointerArray

		for _, location := range tuplesInfo {
			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				return fmt.Errorf("DeleteFromTable: %w", err)
			}
			joinKey := row.Values[rightTableCondition]
			if matchedRow, exists := hashTable[joinKey]; exists {
				query.Result = append(query.Result, matchedRow)
			}

		}
	}

	return nil
}

func CreateTable(parsedQuery *ParsedQuery, query *Query, bpm *storage.BufferPoolManager) error {
	table := parsedQuery.TableReferences[0]
	manager := bpm.DiskScheduler.DiskManager
	tableInfo := storage.TableInfo{Schema: make(map[string]storage.ColumnType)}

	for i := 0; i < len(parsedQuery.ColumnsSelected); i++ {
		columnName := parsedQuery.ColumnsSelected[i]
		columnInfo := parsedQuery.Predicates[i].(storage.ColumnType)

		if columnInfo.IsIndex {
			columnInfo.Type = "INT64"
		}

		tableInfo.Schema[columnName] = columnInfo
	}

	err := manager.CreateTable(storage.TableName(table), tableInfo)
	if err != nil {
		return fmt.Errorf("QueryEngine (CreateTable): %w", err)
	}

	log.Println("TABLE CREATED")
	return nil
}

func GetTable(parsedQuery *ParsedQuery, bpm *storage.BufferPoolManager, step QueryStep) (storage.TableObj, error) {
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

	log.Println("GOT TABLE")
	return *tableObj, err
}

func InsertRows(parsedQuery *ParsedQuery, query *Query, bpm *storage.BufferPoolManager, tableObj *storage.TableObj) error {
	log.Println("INSERTING ROWS")

	var encodedRows [][]byte
	var spaceNeeded int

	rows := parsedQuery.Predicates
	for _, row := range rows {
		rowV2, ok := row.(*storage.RowV2)
		if !ok {
			return fmt.Errorf("InsertRows: row type assertion failed")
		}

		rowV2.ID = storage.GenerateRandomID()
		rowBytes, err := storage.SerializeRow(rowV2)
		if err != nil {
			return fmt.Errorf("InsertRows: serialization error: %w", err)
		}

		encodedRows = append(encodedRows, rowBytes)
		spaceNeeded += len(rowBytes)
	}

	pageFound, err := storage.FindAvailablePage(tableObj.DataFile, spaceNeeded)
	if err != nil {
		return fmt.Errorf("InsertRows: find available page error: %w", err)
	}

	for _, rowBytes := range encodedRows {
		if err := pageFound.AddTuple(rowBytes); err != nil {
			return fmt.Errorf("InsertRows: add tuple error: %w", err)
		}
	}

	manager := bpm.DiskScheduler.DiskManager
	tableName := storage.TableName(parsedQuery.TableReferences[0])
	tableObj, exists := manager.TableObjs[tableName]
	if !exists {
		return fmt.Errorf("InsertRows: table %s not found", tableName)
	}

	pageID := storage.PageID(pageFound.Header.ID)
	pageInfObj, found := tableObj.DirectoryPage.Value[pageID]
	if !found {
		offset, err := manager.WritePageEOFV2(pageFound, tableObj.DataFile)
		if err != nil {
			return fmt.Errorf("InsertRows: write page EOF error: %w", err)
		}

		pageInfObj = &storage.PageInfo{
			Offset:       offset,
			PointerArray: pageFound.PointerArray,
		}

		tableObj.DirectoryPage.Value[pageID] = pageInfObj
		if err := manager.UpdateDirectoryPageDisk(tableObj.DirectoryPage, tableObj.DirFile); err != nil {
			return fmt.Errorf("InsertRows: update directory page error: %w", err)
		}

		err = storage.UpdateBp(rows, *tableObj, *pageInfObj)
		if err != nil {
			return fmt.Errorf("InsertRows: %w", err)
		}

	} else {
		pageInfObj.PointerArray = append(pageInfObj.PointerArray, pageFound.PointerArray...)
		if err := manager.UpdateDirectoryPageDisk(tableObj.DirectoryPage, tableObj.DirFile); err != nil {
			return fmt.Errorf("InsertRows: update directory page error: %w", err)
		}

		if err := manager.WritePageBackV2(pageFound, pageInfObj.Offset, tableObj.DataFile); err != nil {
			return fmt.Errorf("InsertRows: write page back error: %w", err)
		}
		

		err = storage.UpdateBp(rows, *tableObj, *pageInfObj)
		if err != nil {
			return fmt.Errorf("InsertRows: %w", err)
		}

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

func FilterByColumns(tableObj *storage.TableObj, query *Query, P *ParsedQuery) error {
	columnMap := createColumnMap(P.ColumnsSelected)
	pageSlice, err := storage.FullTableScan(tableObj.DataFile)

	if err != nil {
		return fmt.Errorf("FilterByColumns: %w", err)
	}

	for _, page := range pageSlice {
		dirPage := tableObj.DirectoryPage
		pageObj := dirPage.Value[storage.PageID(page.Header.ID)]
		tuplesInfo := pageObj.PointerArray

		for _, location := range tuplesInfo {
			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				return fmt.Errorf("FilterByColumns: %w", err)
			}

			tempTuple := storage.RowV2{Values: make(map[string]string)}

			for key := range columnMap {
				if value, found := row.Values[key]; found {
					tempTuple.Values[key] = value
				}
			}

			query.Result = append(query.Result, &tempTuple)
		}

	}

	return nil
}

func GetAllColumns(tableObj *storage.TableObj, query *Query) error {
	pageSlice, err := storage.FullTableScan(tableObj.DataFile)

	if err != nil {
		return fmt.Errorf("GetAllColumns: %w", err)
	}

	for _, page := range pageSlice {
		dirPage := tableObj.DirectoryPage
		pageObj := dirPage.Value[storage.PageID(page.Header.ID)]
		tuplesInfo := pageObj.PointerArray

		for _, location := range tuplesInfo {
			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				return fmt.Errorf("DeleteFromTable: %w", err)
			}

			query.Result = append(query.Result, row)
		}

	}

	return err
}
