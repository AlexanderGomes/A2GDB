package queryengine

import (
	"crypto/rand"
	"disk-db/storage"
	"fmt"
	"io"
	"math/big"
	"os"
	"strings"
	"unsafe"
)

type Query struct {
	Result  []storage.Row
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
		fmt.Println(err)
		return Query{}, err
	}

	queryPlan, err := GenerateQueryPlan(parsedSQL)
	if err != nil {
		return Query{}, err
	}

	result, _ := qe.ExecuteQueryPlan(queryPlan, parsedSQL)

	return result, nil
}

func (qe *QueryEngine) ExecuteQueryPlan(qp ExecutionPlan, P *ParsedQuery) (Query, error) {
	query := Query{}
	var tableDataFile *os.File
	tablesPtr := []*os.File{}
	tableObj := storage.TableObj{}

	for _, steps := range qp.Steps {
		switch steps.Operation {
		case "GetTable":
			tableObj, tableDataFile = GetTable(P, qe.DB, steps)
		case "GetAllColumns":
			GetAllColumns(tableDataFile, &query)
		case "CollectPointer":
			tablesPtr = append(tablesPtr, tableDataFile)
		case "FilterByColumns":
			FilterByColumns(tableDataFile, &query, P)
		case "InsertRows":
			InsertRows(P, &query, qe.DB, tableDataFile)
		case "CreateTable":
			CreateTable(P, &query, qe.DB)
		case "JoinQueryTable":
			JoinTables(&query, P.Joins[0].Condition, tablesPtr)
		case "DeleteFromTable":
			DeleteFromTable(&query, P, tableDataFile, qe.DB.DiskScheduler.DiskManager, &tableObj)
		case "WhereClause":
			WhereClause(P, &query)
		}
	}

	return query, nil
}

func WhereClause(p *ParsedQuery, q *Query) {
	field := p.Predicates[0].(string)
	condition := p.Predicates[1].(string)
	value := p.Predicates[2].(string)
	res := []storage.Row{}
	if condition == "=" {
		for _, row := range q.Result {
			cleanVal := strings.Trim(row.Values[field], "'")
			if cleanVal == value {
				res = append(res, row)
			}
		}
	}

	q.Result = res
}

func DeleteFromTable(query *Query, p *ParsedQuery, tablePtr *os.File, manager *storage.DiskManagerV2, tableObj *storage.TableObj) {
	tablePages := ReadlAllPages(tablePtr)
	predicateStr := p.Predicates[0].(string)
	comparisonParts := strings.Split(predicateStr, "=")
	field := strings.TrimSpace(comparisonParts[0])
	value := strings.TrimSpace(comparisonParts[1])

	for _, page := range tablePages {
		for _, row := range page.Rows {
			cleanVal := strings.Trim(row.Values[field], "'")
			if cleanVal == value {
				delete(page.Rows, row.ID)
			}
		}

		dirPage := tableObj.DirectoryPage
		offset := dirPage.Mapping[page.ID]
		manager.WritePage(page, offset, tablePtr)
	}
}

func JoinTables(query *Query, condition string, tablePtr []*os.File) {
	slicePage1 := ReadlAllPages(tablePtr[0])
	slicePage2 := ReadlAllPages(tablePtr[1])

	comparisonParts := strings.Split(condition, "=")
	leftTableCondition := strings.TrimSpace(comparisonParts[0])
	rightTableCondition := strings.TrimSpace(comparisonParts[1])

	hashTable := make(map[string]storage.Row)

	for _, page := range slicePage1 {
		for _, row := range page.Rows {
			joinKey := row.Values[leftTableCondition]
			hashTable[joinKey] = row
		}
	}

	for _, page := range slicePage2 {
		for _, row := range page.Rows {
			joinKey := row.Values[rightTableCondition]
			if matchedRow, exists := hashTable[joinKey]; exists {
				query.Result = append(query.Result, matchedRow)
			}
		}
	}
}

func ReadlAllPages(dataFile *os.File) []*storage.Page {
	offset := 0
	pageSlice := []*storage.Page{}

	for {
		page := storage.Page{}
		buffer := make([]byte, storage.PageSize)
		_, err := dataFile.ReadAt(buffer, int64(offset))
		if err != nil && err == io.EOF {
			fmt.Println("readAllPages:end of file, processing pages...")
			break
		}
		storage.DecodeV2(buffer, &page)
		pageSlice = append(pageSlice, &page)
		offset += storage.PageSize
	}

	return pageSlice
}

func CreateTable(parsedQuery *ParsedQuery, query *Query, bpm *storage.BufferPoolManager) {
	table := parsedQuery.TableReferences[0]
	manager := bpm.DiskScheduler.DiskManager
	err := manager.CreateTable(storage.TableName(table), storage.TableInfo{})
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("TABLE CREATED")
}

func GetTable(parsedQuery *ParsedQuery, bpm *storage.BufferPoolManager, step QueryStep) (storage.TableObj, *os.File) {
	manager := bpm.DiskScheduler.DiskManager
	tableNAME := parsedQuery.TableReferences[step.index]

	var tableObj *storage.TableObj
	var err error
	tableObj, found := manager.TableObjs[storage.TableName(tableNAME)]
	if !found {
		tableObj, err = manager.InMemoryTableSetUp(storage.TableName(tableNAME))
		if err != nil {
			fmt.Println(err)
			return storage.TableObj{}, nil
		}
	}

	fmt.Println("GOT TABLE")
	return *tableObj, tableObj.DataFile
}

func InsertRows(parsedQuery *ParsedQuery, query *Query, bpm *storage.BufferPoolManager, tablePtr *os.File) {
	fmt.Println("INSERTING")
	rows := parsedQuery.Predicates[0].(storage.Row)
	updatedPage := FindAvailablePage(tablePtr, parsedQuery, &rows)

	manager := bpm.DiskScheduler.DiskManager
	tableObj := manager.TableObjs[storage.TableName(parsedQuery.TableReferences[0])]

	offset, found := tableObj.DirectoryPage.Mapping[updatedPage.ID]

	// # just created the page
	if !found {
		pageReq := storage.DiskReq{
			Page:      *updatedPage,
			Operation: "WRITE",
		}

		offset, err := manager.CreatePage(pageReq, tableObj)
		if err != nil {
			errWrap := fmt.Errorf("Query error writing page to data file: %w", err)
			fmt.Println(errWrap)
			return
		}

		tableObj.DirectoryPage.Mapping[updatedPage.ID] = offset
		err = manager.UpdateDirectoryPageDisk(tableObj.DirectoryPage, tableObj)
		if err != nil {
			errWrap := fmt.Errorf("Query error updating directory page: %w", err)
			fmt.Println(errWrap)
			return
		}

		return
	}

	// # don't update table directory page
	err := manager.WritePage(updatedPage, offset, tableObj.DataFile)
	if err != nil {
		errWrap := fmt.Errorf("Query error writing EXISTING page to data file: %w", err)
		fmt.Println(errWrap)
	}
}

// # does this function belong here, or with disk manager ?
func FindAvailablePage(tablePtr *os.File, parsedQuery *ParsedQuery, rows *storage.Row) *storage.Page {
	offset := 0
	// not necessary for decoding only for creating
	page := storage.Page{Rows: make(map[uint64]storage.Row)}

	for {
		pageBytes := make([]byte, storage.PageSize)
		_, err := tablePtr.ReadAt(pageBytes, int64(offset))
		if err != nil {
			if err == io.EOF {
				fmt.Println("FindAvailablePage: End of file reached, creating new page")
				CreatePage(&page, rows, parsedQuery.TableReferences[0])
				return &page
			}
			fmt.Printf("FindAvailablePage: Query error reading a page: %v\n", err)
			return nil
		}

		offset += storage.PageSize
		storage.DecodeV2(pageBytes, &page)

		cleanSize := getSizeOfIDAndRows(&page)

		if storage.PageSize > cleanSize {
			fmt.Println("true")
			page.TABLE = parsedQuery.TableReferences[0]
			rows.ID = generateRandomID()
			page.Rows[rows.ID] = *rows
			break
		}

		page = storage.Page{Rows: make(map[uint64]storage.Row)}

	}

	return &page
}

func CreatePage(page *storage.Page, rows *storage.Row, tableName string) {
	page.Rows = make(map[uint64]storage.Row)
	pageID := generateRandomID()
	page.ID = storage.PageID(pageID)
	page.TABLE = tableName

	rowID := generateRandomID()
	rows.ID = rowID
	page.Rows[rows.ID] = *rows
}

func createColumnMap(columns []string) map[string]string {
	columnMap := make(map[string]string)

	for _, name := range columns {
		columnMap[name] = name
	}

	return columnMap
}

func FilterByColumns(filePtr *os.File, query *Query, P *ParsedQuery) {
	columnMap := createColumnMap(P.ColumnsSelected)

	offset := 0
	pageSlice := []*storage.Page{}

	for {
		page := storage.Page{}
		buffer := make([]byte, storage.PageSize)
		_, err := filePtr.ReadAt(buffer, int64(offset))
		if err != nil && err == io.EOF {
			fmt.Println("FilterByColumns: end of file, processing pages...")
			break
		}
		storage.DecodeV2(buffer, &page)
		pageSlice = append(pageSlice, &page)
		offset += storage.PageSize
	}

	for _, page := range pageSlice {
		for _, tuple := range page.Rows {
			for key := range tuple.Values {
				if _, found := columnMap[key]; !found {
					delete(tuple.Values, key)
				}
			}

			query.Result = append(query.Result, tuple)
		}
	}
}

func GetAllColumns(filePtr *os.File, query *Query) {
	offset := 0
	pageSlice := []*storage.Page{}

	for {
		page := storage.Page{}
		buffer := make([]byte, storage.PageSize)
		_, err := filePtr.ReadAt(buffer, int64(offset))
		if err != nil && err == io.EOF {
			fmt.Println("gerAllColumns: end of file, processing pages...")
			break
		}
		storage.DecodeV2(buffer, &page)
		pageSlice = append(pageSlice, &page)
		offset += storage.PageSize
	}

	for _, page := range pageSlice {
		for _, tuple := range page.Rows {
			query.Result = append(query.Result, tuple)
		}
	}
}

func generateRandomID() uint64 {
	max := new(big.Int).Lsh(big.NewInt(1), 64) // 2^64
	randomNum, _ := rand.Int(rand.Reader, max)

	return randomNum.Uint64()
}

func getSizeOfIDAndRows(page *storage.Page) uintptr {
	size := unsafe.Sizeof(page.ID) // Size of PageID

	// Calculate size of map header
	size += unsafe.Sizeof(page.Rows) // Size of the map header

	// Calculate size of map keys and values
	for k, v := range page.Rows {
		size += unsafe.Sizeof(k) // Size of the key (uint64)
		size += unsafe.Sizeof(v) // Size of the value (Row)

		// Size of Row.Values (map[string]string)
		for key, value := range v.Values {
			size += unsafe.Sizeof(key)   // Size of the key (string header)
			size += uintptr(len(key))    // Size of the string data for key
			size += unsafe.Sizeof(value) // Size of the value (string header)
			size += uintptr(len(value))  // Size of the string data for value
		}
	}

	return size
}
