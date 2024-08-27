package queryengine

import (
	"disk-db/storage"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

const MAX_FILE_SIZE = 100 * 1024

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
	var offset *storage.Offset

	query := Query{}
	tablesPtr := []*storage.TableObj{}
	var tableObj *storage.TableObj
	var groupByMap map[string][]string
	var resMap interface{}

	for _, steps := range qp.Steps {
		switch steps.Operation {
		case "GetTable":
			tableObj, err = GetTable(P, qe.DB, steps)
		case "GetAllColumns":
			err = GetAllColumns(P, tableObj, &query, offset)
		case "CollectPointer":
			tablesPtr = append(tablesPtr, tableObj)
		case "FilterByColumns":
			err = FilterByColumns(tableObj, &query, P, offset)
		case "InsertRows":
			err = InsertRows(P, &query, qe.DB, tableObj)
		case "CreateTable":
			err = CreateTable(P, &query, qe.DB)
		case "JoinQueryTable":
			err = JoinTables(&query, P.Joins[0].Condition, tablesPtr)
		case "DeleteFromTable":
			err = DeleteFromTable(P, tableObj, offset)
		case "Update":
			err = Update(P, qe.DB.DiskScheduler.DiskManager, tableObj, offset)
		case "DetermineScan":
			offset, err = DetermineScan(P, qe.DB.DiskScheduler.DiskManager)
		case "GroupByColumn":
			groupByMap, err = GroupByColumn(P, tableObj)
		case "GroupByFunction":
			resMap, err = GroupByFunction(P, groupByMap)
		case "CollectGroupBy":
			CollectGroupBy(resMap, &query, P)
		case "OrderBy":
			OrderByExecution(&query, P)
		}
		if err != nil {
			return Query{}, fmt.Errorf("ExecuteQueryPlan: %w", err)
		}
	}

	return query, nil
}

func OrderByExecution(q *Query, p *ParsedQuery) {
	switch p.OrderBy.Operation {
	case "desc":
		Desc(q, p.OrderBy.Column)
	case "asc":
		Asc(q, p.OrderBy.Column)
	}
}

func Desc(q *Query, column string) {
	rows := q.Result
	sort.Slice(rows, func(i, j int) bool {
		valI, okI := rows[i].Values[column]
		valJ, okJ := rows[j].Values[column]

		if !okI || !okJ {
			return false
		}

		numI, errI := strconv.ParseFloat(valI, 64)
		numJ, errJ := strconv.ParseFloat(valJ, 64)

		if errI != nil || errJ != nil {
			return valI > valJ
		}

		return numI > numJ
	})
}

func Asc(q *Query, column string) {
	rows := q.Result
	sort.Slice(rows, func(i, j int) bool {
		valI, okI := rows[i].Values[column]
		valJ, okJ := rows[j].Values[column]

		if !okI || !okJ {
			return false
		}

		numI, errI := strconv.ParseFloat(valI, 64)
		numJ, errJ := strconv.ParseFloat(valJ, 64)

		if errI != nil || errJ != nil {
			return valI < valJ
		}

		return numI < numJ
	})
}

func CollectGroupBy(resMap interface{}, query *Query, p *ParsedQuery) {
	switch p.SelectFunc.FuncName {
	case "AVG":
		floatMap := resMap.(map[string]float64)
		for k, v := range floatMap {
			row := storage.RowV2{Values: map[string]string{}}
			row.Values[k] = fmt.Sprintf("%f", v)
			query.Result = append(query.Result, &row)
		}

	case "SUM", "MAX", "MIN", "COUNT":
		intMap := resMap.(map[string]int64)
		for k, v := range intMap {
			row := storage.RowV2{Values: map[string]string{}}
			row.Values[k] = fmt.Sprintf("%d", v)
			query.Result = append(query.Result, &row)
		}
	}
}

func GroupByFunction(p *ParsedQuery, groupMap map[string][]string) (interface{}, error) {
	var resMap interface{}
	var err error

	switch p.SelectFunc.FuncName {
	case "AVG":
		resMap, err = Average(groupMap)
		if err != nil {
			return nil, fmt.Errorf("GroupByFunction: %w", err)
		}
	case "SUM":
		resMap, err = Sum(groupMap)
		if err != nil {
			return nil, fmt.Errorf("GroupByFunction: %w", err)
		}
	case "MAX":
		resMap, err = Max(groupMap)
		if err != nil {
			return nil, fmt.Errorf("GroupByFunction: %w", err)
		}
	case "MIN":
		resMap, err = Min(groupMap)
		if err != nil {
			return nil, fmt.Errorf("GroupByFunction: %w", err)
		}

	case "COUNT":
		resMap, err = Count(groupMap)
		if err != nil {
			return nil, fmt.Errorf("GroupByFunction: %w", err)
		}
	}

	return resMap, nil
}

func Count(groupMap map[string][]string) (map[string]int64, error) {
	countMap := make(map[string]int64)
	for key, vals := range groupMap {
		countMap[key] = int64(len(vals))
	}

	return countMap, nil
}

func Min(groupMap map[string][]string) (map[string]int64, error) {
	minMap := make(map[string]int64)

	for key, vals := range groupMap {
		var min int64 = math.MaxInt64

		for _, val := range vals {
			num, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("GroupByFunction: %w", err)
			}

			if num < min {
				min = num
			}
		}

		minMap[key] = min
	}

	return minMap, nil
}

func Max(groupMap map[string][]string) (map[string]int64, error) {
	maxMap := make(map[string]int64)

	for key, vals := range groupMap {
		var max int64 = math.MinInt64

		for _, val := range vals {
			num, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("GroupByFunction: %w", err)
			}

			if num > max {
				max = num
			}
		}

		maxMap[key] = max
	}

	return maxMap, nil
}

func Sum(groupMap map[string][]string) (map[string]int64, error) {
	sumMap := make(map[string]int64)

	for key, vals := range groupMap {
		var sum int64

		for _, val := range vals {
			num, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("GroupByFunction(string => float error): %w", err)
			}
			sum += num
		}

		sumMap[key] = sum
	}

	return sumMap, nil
}

func Average(groupMap map[string][]string) (map[string]float64, error) {
	avgMap := make(map[string]float64)

	for key, vals := range groupMap {
		var sum float64
		var count int

		for _, val := range vals {
			num, err := strconv.ParseFloat(val, 64)
			if err != nil {
				return nil, fmt.Errorf("GroupByFunction(string => float error): %w", err)
			}
			sum += num
			count++
		}

		if count > 0 {
			avgMap[key] = sum / float64(count)
		}
	}

	return avgMap, nil
}

func GroupByColumn(p *ParsedQuery, tableObj *storage.TableObj) (map[string][]string, error) {
	pageSlice, err := getTablePages(tableObj.DataFile, nil)
	if err != nil {
		return nil, fmt.Errorf("GroupByColumn: %w", err)
	}

	hashmap := make(map[string][]string)
	field := p.GroupBy
	value := p.SelectFunc.FuncParameter

	for _, page := range pageSlice {
		pageObj := tableObj.DirectoryPage.Value[storage.PageID(page.Header.ID)]

		for _, location := range pageObj.PointerArray {
			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				return nil, fmt.Errorf("processPagesUpdate: failed to decode row: %w", err)
			}

			groupKey := row.Values[field]
			groupVal := row.Values[value]

			hashmap[groupKey] = append(hashmap[groupKey], groupVal)
		}

	}
	return hashmap, nil
}

func DetermineScan(p *ParsedQuery, dm *storage.DiskManagerV2) (*storage.Offset, error) {
	if len(p.Where) == 0 {
		return nil, nil
	}

	whereField := p.Where[0]
	whereValue := p.Where[1]

	tableName := storage.TableName(p.TableReferences[0])
	tableInfo := dm.PageCatalog.Tables[tableName]
	columnType := tableInfo.Schema[whereField]

	if columnType.IsIndex {
		tableObj := dm.TableObjs[tableName]

		uintValue, err := strconv.ParseUint(whereValue, 10, 64)
		if err != nil {
			log.Println("DetermineScan (The value isn't a primary key)")
			return nil, nil
		}

		item, err := storage.GetItemByKey(tableObj.BpTree, uintValue)
		if err != nil {
			log.Println("DetermineScan (wrong primary key)")
			return nil, nil
		}

		return &item.Value, nil
	}

	return nil, nil
}

func Update(p *ParsedQuery, manager *storage.DiskManagerV2, tableObj *storage.TableObj, offset *storage.Offset) error {
	var tablePages []*storage.PageV2
	var err error

	tablePages, err = getTablePages(tableObj.DataFile, offset)
	if err != nil {
		return fmt.Errorf("UPDATE: %w", err)
	}

	findField := p.Where[0]
	findValue := p.Where[1]

	changingField := p.Predicates[0].(string)
	newValue := p.Predicates[1].(string)

	if err := processPagesUpdate(tablePages, findField, findValue, changingField, newValue, tableObj); err != nil {
		return fmt.Errorf("UPDATE: %w", err)
	}

	if err := writeUpdatedPages(tablePages, tableObj); err != nil {
		return fmt.Errorf("UPDATE: %w", err)
	}

	err = storage.UpdateDirectoryPageDisk(tableObj.DirectoryPage, tableObj.DirFile)
	if err != nil {
		return fmt.Errorf("UPDATE: %w", err)
	}

	return nil
}

func getTablePages(dataFile *os.File, offset *storage.Offset) ([]*storage.PageV2, error) {
	stat, _ := dataFile.Stat()
	size := stat.Size()

	if offset == nil {
		if size >= MAX_FILE_SIZE {
			return storage.FullTableScanBigFiles(dataFile)
		}
		return storage.FullTableScan(dataFile)
	}

	bytes, err := storage.ReadPageAtOffset(dataFile, *offset)
	if err != nil {
		return nil, fmt.Errorf("getTablePages: %w", err)
	}

	page, err := storage.DecodePageV2(bytes)
	if err != nil {
		return nil, fmt.Errorf("getTablePages: %w", err)
	}

	log.Println("Index Scan")
	return []*storage.PageV2{page}, nil
}

func processPagesUpdate(pages []*storage.PageV2, findField, findValue, changingField, newValue string, tableObj *storage.TableObj) error {
	for _, page := range pages {
		var newPtrArray []storage.TupleLocation
		pageObj := tableObj.DirectoryPage.Value[storage.PageID(page.Header.ID)]

		for i := range pageObj.PointerArray {
			location := &pageObj.PointerArray[i]
			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				return fmt.Errorf("processPagesUpdate: failed to decode row: %w", err)
			}

			if row.Values[findField] == findValue {
				row.Values[changingField] = newValue
				updatedBytes, err := storage.SerializeRow(row)
				if err != nil {
					return fmt.Errorf("processPagesUpdate: failed to serialize row: %w", err)
				}

				updatedTupleLength := len(updatedBytes)

				pageObj.FSM = append(pageObj.FSM, i)
				location.Free = false

				err = storage.ResetBytesToEmpty(page, location.Offset, location.Length)
				if err != nil {
					return fmt.Errorf("processPagesUpdate: failed to reset bytes: %w", err)
				}

				// may make create fragmentation adding a smaller tuple to the same place
				if updatedTupleLength <= int(location.Length) {
					copy(page.Data[location.Offset:], updatedBytes)
					location.Length = uint16(updatedTupleLength)
					newPtrArray = append(newPtrArray, *location)
				} else {
					// #Edge case: new tuple may be too big for the page
					for _, index := range pageObj.FSM {
						freeSpace := &pageObj.PointerArray[index]

						if freeSpace.Length >= uint16(updatedTupleLength) {
							copy(page.Data[freeSpace.Offset:], updatedBytes)
							freeSpace.Length = uint16(updatedTupleLength)
							freeSpace.Free = false
							pageObj.FSM = append(pageObj.FSM[:i], pageObj.FSM[i+1:]...)
							return nil
						}
					}

					err = page.AddTuple(updatedBytes)
					if err != nil {
						return fmt.Errorf("processPagesUpdate: failed to add updated tuple: %w", err)
					}

					newPtrArray = append(newPtrArray, page.PointerArray[len(page.PointerArray)-1])
				}
			}
		}

		pageObj.PointerArray = append(pageObj.PointerArray, newPtrArray...)
	}

	return nil
}

func writeUpdatedPages(pages []*storage.PageV2, tableObj *storage.TableObj) error {
	for _, page := range pages {
		pageObj := tableObj.DirectoryPage.Value[storage.PageID(page.Header.ID)]
		if err := storage.WritePageBackV2(page, pageObj.Offset, tableObj.DataFile); err != nil {
			return fmt.Errorf("writeUpdatedPages: %w", err)
		}
	}
	return nil
}

func DeleteFromTable(p *ParsedQuery, tableObj *storage.TableObj, offset *storage.Offset) error {
	var tablePages []*storage.PageV2
	var err error

	tablePages, err = getTablePages(tableObj.DataFile, offset)
	if err != nil {
		return fmt.Errorf("DELETE: %w", err)
	}

	field := p.Where[0]
	value := p.Where[1]

	if err := processPagesForDeletion(tablePages, field, value, tableObj); err != nil {
		return fmt.Errorf("DELETE: %w", err)
	}

	if err := writeUpdatedPages(tablePages, tableObj); err != nil {
		return fmt.Errorf("DELETE: %w", err)
	}

	if err = storage.UpdateDirectoryPageDisk(tableObj.DirectoryPage, tableObj.DirFile); err != nil {
		return fmt.Errorf("DELETE: %w", err)
	}

	return nil
}

func processPagesForDeletion(pages []*storage.PageV2, field, value string, tableObj *storage.TableObj) error {
	for _, page := range pages {
		pageObj := tableObj.DirectoryPage.Value[storage.PageID(page.Header.ID)]
		for i := range pageObj.PointerArray {
			location := &pageObj.PointerArray[i]
			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				return fmt.Errorf("processPagesForDeletion: %w", err)
			}

			if row.Values[field] == value {
				location.Free = true
				pageObj.FSM = append(pageObj.FSM, i)
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

	slicePage1, err := getTablePages(tablePtrs[0].DataFile, nil)
	if err != nil {
		return fmt.Errorf("JoinTables (error reading table one): %w", err)
	}

	slicePage2, err := getTablePages(tablePtrs[0].DataFile, nil)
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

func GetTable(parsedQuery *ParsedQuery, bpm *storage.BufferPoolManager, step QueryStep) (*storage.TableObj, error) {
	log.Println("GETTING TABLE")

	manager := bpm.DiskScheduler.DiskManager
	tableNAME := parsedQuery.TableReferences[step.index]

	var tableObj *storage.TableObj
	var err error

	tableObj, found := manager.TableObjs[storage.TableName(tableNAME)]
	if !found {
		tableObj, err = manager.InMemoryTableSetUp(storage.TableName(tableNAME))
		if err != nil {
			return nil, fmt.Errorf("GetTable: %w", err)
		}
	}

	return tableObj, err
}

func InsertRows(parsedQuery *ParsedQuery, query *Query, bpm *storage.BufferPoolManager, tableObj *storage.TableObj) error {
	log.Println("INSERTING ROWS")

	catalog := bpm.DiskScheduler.DiskManager.PageCatalog
	encodedRows, spaceNeeded, err := serializeRows(parsedQuery.Predicates, catalog, parsedQuery.TableReferences[0])
	if err != nil {
		return fmt.Errorf("InsertRows: %w", err)
	}

	pageFound, err := storage.FindAvailablePage(tableObj, spaceNeeded)
	if err != nil {
		return fmt.Errorf("InsertRows: %w", err)
	}

	if err := addRowsToPage(pageFound, encodedRows); err != nil {
		return fmt.Errorf("InsertRows: %w", err)
	}

	pageID := storage.PageID(pageFound.Header.ID)
	if err := updatePageInfo(pageID, pageFound, tableObj, parsedQuery); err != nil {
		return fmt.Errorf("InsertRows: %w", err)
	}

	return nil
}

func serializeRows(rows []interface{}, catalog *storage.Catalog, tableName interface{}) ([][]byte, int, error) {
	var encodedRows [][]byte
	var spaceNeeded int

	tableNameStr := tableName.(string)
	tableInfo := catalog.Tables[storage.TableName(tableNameStr)]
	var primaryIdField string

	for key, val := range tableInfo.Schema {
		if val.IsIndex {
			primaryIdField = key
		}
	}

	for _, row := range rows {
		rowV2, ok := row.(*storage.RowV2)
		if !ok {
			return nil, 0, fmt.Errorf("row type assertion failed")
		}

		rowV2.ID = storage.GenerateRandomID()
		rowV2.Values[primaryIdField] = strconv.FormatUint(rowV2.ID, 10)

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

func updatePageInfo(pageID storage.PageID, pageFound *storage.PageV2, tableObj *storage.TableObj, pq *ParsedQuery) error {
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
		if err := storage.UpdateDirectoryPageDisk(dirPage, tableObj.DirFile); err != nil {
			return fmt.Errorf("update directory page error: %w", err)
		}
	} else {
		pageInfObj.PointerArray = append(pageInfObj.PointerArray, pageFound.PointerArray...)
		if err := storage.UpdateDirectoryPageDisk(dirPage, tableObj.DirFile); err != nil {
			return fmt.Errorf("update directory page error: %w", err)
		}

		if err := storage.WritePageBackV2(pageFound, pageInfObj.Offset, tableObj.DataFile); err != nil {
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

func FilterByColumns(tableObj *storage.TableObj, query *Query, P *ParsedQuery, offset *storage.Offset) error {
	pageSlice, err := getTablePages(tableObj.DataFile, offset)
	if err != nil {
		return fmt.Errorf("GetAllColumns: %w", err)
	}

	columnMap := createColumnMap(P.ColumnsSelected)
	dirPage := tableObj.DirectoryPage
	dirPageValues := dirPage.Value
	hasWhereClause := len(P.Where) > 0
	var field, value string
	if hasWhereClause {
		field, value = P.Where[0], P.Where[1]
	}

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

			if !hasWhereClause || row.Values[field] == value {
				tempTuple := storage.RowV2{Values: make(map[string]string)}
				for col := range columnMap {
					if value, found := row.Values[col]; found {
						tempTuple.Values[col] = value
					}
				}
				query.Result = append(query.Result, &tempTuple)
			}

		}
	}

	return nil
}

func GetAllColumns(p *ParsedQuery, tableObj *storage.TableObj, query *Query, offset *storage.Offset) error {
	pageSlice, err := getTablePages(tableObj.DataFile, offset)
	if err != nil {
		return fmt.Errorf("GetAllColumns: %w", err)
	}

	dirPageValues := tableObj.DirectoryPage.Value
	hasWhereClause := len(p.Where) > 0

	var field, value string
	if hasWhereClause {
		field, value = p.Where[0], p.Where[1]
	}

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

			if !hasWhereClause || row.Values[field] == value {
				query.Result = append(query.Result, row)
			}
		}
	}

	return nil
}
