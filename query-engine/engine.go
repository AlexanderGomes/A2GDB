package queryengine

import (
	"disk-db/logger"
	"disk-db/storage"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
)

type Query struct {
	Result  []*storage.RowV2
	Message string
}

type QueryEngine struct {
	Disk *storage.DiskManagerV2
}

func (qe *QueryEngine) ExecuteQueryPlan(qp ExecutionPlan, P *ParsedQuery) (Query, error) {
	var err error
	var offset *storage.Offset

	query := Query{}
	filteredJoinRows := [][]*storage.RowV2{}
	var tableObj *storage.TableObj
	var groupByMap map[string][]string
	var resMap interface{}

	for _, steps := range qp.Steps {
		switch steps.Operation {
		case "GetTable":
			tableObj, err = GetTable(P, qe.Disk, steps)
		case "GetAllColumns":
			err = GetAllColumns(P, tableObj, &query, offset)
		case "CollectData":
			filteredJoinRows = append(filteredJoinRows, query.Result)
			query.Result = []*storage.RowV2{}
		case "FilterByColumns":
			err = FilterByColumns(tableObj, &query, P, offset, steps)
		case "InsertRows":
			err = InsertRows(P, &query, qe.Disk, tableObj)
		case "CreateTable":
			err = CreateTable(P, &query, qe.Disk)
		case "JoinQueryTable":
			err = JoinTables(&query, P.Joins.Condition, filteredJoinRows)
		case "DeleteFromTable":
			err = DeleteFromTable(P, tableObj, offset)
		case "Update":
			err = Update(P, qe.Disk, tableObj, offset)
		case "DetermineScan":
			offset, err = DetermineScan(P, qe.Disk)
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
			return Query{}, err
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
	pageSlice, err := storage.GetTablePages(tableObj.DataFile, nil)
	if err != nil {
		return nil, fmt.Errorf("GroupByColumn: %w", err)
	}

	hashmap := make(map[string][]string)
	field := p.GroupBy
	value := p.SelectFunc.FuncParameter

	for _, page := range pageSlice {
		pageObj := tableObj.DirectoryPage.Value[storage.PageID(page.Header.ID)]

		for _, location := range pageObj.PointerArray {
			if location.Free {
				continue
			}

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

	tablePages, err = storage.GetTablePages(tableObj.DataFile, offset)
	if err != nil {
		return fmt.Errorf("UPDATE: %w", err)
	}

	findField := p.Where[0]
	findValue := p.Where[1]

	changingField := p.Predicates[0].(string)
	newValue := p.Predicates[1].(string)

	err = processPagesUpdate(&tablePages, findField, findValue, changingField, newValue, tableObj)
	if err != nil {
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

func processPagesUpdate(pages *[]*storage.PageV2, findField, findValue, changingField, newValue string, tableObj *storage.TableObj) error {
	pageHash := BuildPageHash(pages)

	for _, page := range *pages {
		var newPtrArray []storage.TupleLocation
		pageObj := tableObj.DirectoryPage.Value[storage.PageID(page.Header.ID)]

		for i := range pageObj.PointerArray {
			location := &pageObj.PointerArray[i]
			if location.Free {
				continue
			}

			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				return fmt.Errorf("processPagesUpdate: failed to decode row: %w", err)
			}

			if row.Values[findField] == findValue {
				updatedBytes, updatedTupleLength, err := UpdateTuple(row, changingField, newValue)
				if err != nil {
					return fmt.Errorf("processPagesUpdate: %w", err)
				}

				err = CleanOldTupleSpace(pageObj, tableObj, page, location, i)
				if err != nil {
					return fmt.Errorf("processPagesUpdate: %w", err)
				}

				err = page.AddTuple(updatedBytes)
				if err != nil {
					item := storage.Item{
						Key: row.ID,
					}

					deletedItem := tableObj.BpTree.Delete(item)

					if deletedItem == nil {
						return errors.New("item not deleted")
					}

					err := InsertBigTuple(row, updatedBytes, updatedTupleLength, tableObj, pageHash)
					if err != nil {
						return fmt.Errorf("processPagesUpdate: %w", err)
					}
					continue
				}

				newPtrArray = append(newPtrArray, page.PointerArray[len(page.PointerArray)-1])
			}
		}

		pageObj.PointerArray = append(pageObj.PointerArray, newPtrArray...)
	}

	return nil
}

func InsertBigTuple(row *storage.RowV2, rowBytes []byte, rowLength int, tableObj *storage.TableObj, pageHash map[uint64]*storage.PageV2) error {
	if rowLength >= storage.PageSizeV2 {
		return errors.New("InsetBigTuple: row is bigger than pagesize")
	}

	createNewPage := true
	page, err := storage.FindAvailablePage(tableObj.DataFile, rowLength, createNewPage)
	if err != nil {
		return fmt.Errorf("InsetBigTuple: %w", err)
	}
	page.AddTuple(rowBytes)

	err = UpdateDiskNewPage(page, tableObj, tableObj.DirectoryPage, row)
	if err != nil {
		return fmt.Errorf("InsetBigTuple: %w", err)
	}

	return nil
}

func UpdateDiskNewPage(page *storage.PageV2, tableObj *storage.TableObj, dirPage *storage.DirectoryPageV2, row *storage.RowV2) error {
	offset, err := storage.WritePageEOFV2(page, tableObj.DataFile)
	if err != nil {
		return fmt.Errorf("UpdateDisk: %w", err)
	}

	newPageObj := storage.PageInfo{
		Offset:       offset,
		PointerArray: page.PointerArray,
	}

	dirPage.Value[storage.PageID(page.Header.ID)] = &newPageObj
	err = storage.UpdateDirectoryPageDisk(tableObj.DirectoryPage, tableObj.DirFile)
	if err != nil {
		return fmt.Errorf("UpdateDisk: %w", err)
	}

	itemInsert := storage.Item{
		Key:   row.ID,
		Value: offset,
	}

	tableObj.BpTree.ReplaceOrInsert(itemInsert)
	err = UpdateBp(tableObj)
	if err != nil {
		return fmt.Errorf("UpdateDisk: %w", err)
	}

	return nil
}

func BuildPageHash(pages *[]*storage.PageV2) map[uint64]*storage.PageV2 {
	pageMap := make(map[uint64]*storage.PageV2)
	for _, page := range *pages {
		pageMap[page.Header.ID] = page
	}

	return pageMap
}

func CleanOldTupleSpace(pageObj *storage.PageInfo, tableObj *storage.TableObj, page *storage.PageV2, location *storage.TupleLocation, i int) error {
	location.Free = true
	pageObj.FSM = append(pageObj.FSM, i)

	err := storage.ResetBytesToEmpty(page, location.Offset, location.Length)
	if err != nil {
		return fmt.Errorf("CleanOldTupleSpace: failed to reset bytes: %w", err)
	}

	return nil
}

func UpdateTuple(row *storage.RowV2, changingField, newValue string) ([]byte, int, error) {
	row.Values[changingField] = newValue
	updatedBytes, err := storage.SerializeRow(row)
	if err != nil {
		return nil, 0, fmt.Errorf("UpdatingTuple: failed to serialize row: %w", err)
	}

	updatedTupleLength := len(updatedBytes)
	if updatedTupleLength > storage.PageDataSize {
		return nil, 0, errors.New("UpdatingTuple: tuple is bigger than page")
	}

	return updatedBytes, updatedTupleLength, nil
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

	tablePages, err = storage.GetTablePages(tableObj.DataFile, offset)
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

	if err := storage.UpdateDirectoryPageDisk(tableObj.DirectoryPage, tableObj.DirFile); err != nil {
		return fmt.Errorf("DELETE: %w", err)
	}

	if err := UpdateBp(tableObj); err != nil {
		return fmt.Errorf("DELETE: %w", err)
	}

	return nil
}

func UpdateBp(tableObj *storage.TableObj) error {
	items := storage.GetAllItems(tableObj.BpTree)
	bytes, err := storage.EncodeItems(items)
	if err != nil {
		return fmt.Errorf("UpdateBp: %w", err)
	}

	err = storage.WriteNonPageFile(tableObj.BpFile, bytes)
	if err != nil {
		return fmt.Errorf("UpdateBp: %w", err)
	}

	return nil
}

func processPagesForDeletion(pages []*storage.PageV2, field, value string, tableObj *storage.TableObj) error {
	for _, page := range pages {
		pageObj := tableObj.DirectoryPage.Value[storage.PageID(page.Header.ID)]
		for i := range pageObj.PointerArray {
			location := &pageObj.PointerArray[i]
			if location.Free {
				continue
			}

			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				return fmt.Errorf("processPagesForDeletion: %w", err)
			}

			if row.Values[field] == value {
				location.Free = true
				pageObj.FSM = append(pageObj.FSM, i)
				storage.ResetBytesToEmpty(page, location.Offset, location.Length)

				itemToDelete := storage.Item{
					Key: row.ID,
				}

				item := tableObj.BpTree.Delete(itemToDelete)

				if item == nil {
					return errors.New("bptree item not found")
				}
			}
		}
	}
	return nil
}

func JoinTables(query *Query, condition Condition, filteredRows [][]*storage.RowV2) error {
	for _, rowLeft := range filteredRows[0] {
		for _, rowRight := range filteredRows[1] {
			if rowLeft.Values[condition.Left] == rowRight.Values[condition.Right] {
				query.Result = append(query.Result, rowLeft, rowRight)
			}
		}
	}

	return nil
}

func CreateTable(parsedQuery *ParsedQuery, query *Query, manager *storage.DiskManagerV2) error {
	tableName := parsedQuery.TableReferences[0]

	_, ok := manager.PageCatalog.Tables[storage.TableName(tableName)]
	if ok {
		return nil
	}

	tableSchema, err := buildTableSchema(parsedQuery)
	if err != nil {
		return fmt.Errorf("CreateTable (error building schema): %w", err)
	}

	if err := manager.CreateTable(storage.TableName(tableName), tableSchema); err != nil {
		return err
	}

	logger.Log.Info("TABLE CREATED")
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

func GetTable(parsedQuery *ParsedQuery, manager *storage.DiskManagerV2, step QueryStep) (*storage.TableObj, error) {
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

	logger.Log.Infof("Got Table Object for %v", tableNAME)
	return tableObj, err
}

func InsertRows(parsedQuery *ParsedQuery, query *Query, manager *storage.DiskManagerV2, tableObj *storage.TableObj) error {
	catalog := manager.PageCatalog
	encodedRows, spaceNeeded, err := serializeRows(parsedQuery.Predicates, catalog, parsedQuery.TableReferences[0])
	if err != nil {
		return fmt.Errorf("InsertRows: %w", err)
	}

	pageFound, err := storage.FindAvailablePage(tableObj.DataFile, spaceNeeded, false)
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

	logger.Log.Info("Inserted Rows")
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

		if spaceNeeded > storage.PageDataSize {
			return nil, 0, fmt.Errorf("serializeRows: space needed bigger than a single page")
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
	} else {
		pageInfObj.PointerArray = append(pageInfObj.PointerArray, pageFound.PointerArray...)
		if err := storage.WritePageBackV2(pageFound, pageInfObj.Offset, tableObj.DataFile); err != nil {
			return fmt.Errorf("write page back error: %w", err)
		}
	}

	if err := storage.UpdateDirectoryPageDisk(dirPage, tableObj.DirFile); err != nil {
		return fmt.Errorf("update directory page error: %w", err)
	}

	if err := storage.UpdateBp(pq.Predicates, *tableObj, *pageInfObj); err != nil {
		return fmt.Errorf("update B+ tree error: %w", err)
	}

	return nil
}

func FilterByColumns(tableObj *storage.TableObj, query *Query, P *ParsedQuery, offset *storage.Offset, step QueryStep) error {
	pageSlice, err := storage.GetTablePages(tableObj.DataFile, offset)
	if err != nil {
		return fmt.Errorf("GetAllColumns: %w", err)
	}

	columns := P.ColumnsSelected
	dirPage := tableObj.DirectoryPage
	dirPageValues := dirPage.Value

	hasWhereClause := len(P.Where) > 0
	var field, value string
	if hasWhereClause {
		field, value = P.Where[0], P.Where[1]
	}

	if P.Joins != nil {
		tableName := P.TableReferences[step.index]
		columns = P.Joins.TableColumns[tableName]

		if step.index == 0 {
			columns = append(columns, P.Joins.Condition.Left)
		} else {
			columns = append(columns, P.Joins.Condition.Right)
		}
	}

	for _, page := range pageSlice {
		pageID := storage.PageID(page.Header.ID)
		pageObj, exists := dirPageValues[pageID]
		if !exists {
			return fmt.Errorf("FilterByColumns: page info not found for page ID %d", pageID)
		}

		for _, location := range pageObj.PointerArray {
			if location.Free {
				continue
			}

			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				return fmt.Errorf("FilterByColumns: failed to decode row: %w", err)
			}

			if !hasWhereClause || row.Values[field] == value {
				tempTuple := storage.RowV2{Values: make(map[string]string)}
				for _, col := range columns {
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
	pageSlice, err := storage.GetTablePages(tableObj.DataFile, offset)
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
			if location.Free {
				continue
			}

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
