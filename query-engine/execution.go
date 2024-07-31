package queryengine

import (
	"crypto/rand"
	"disk-db/storage"
	"fmt"
	"io"
	"math/big"
	"os"
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

	for _, steps := range qp.Steps {
		switch steps.Operation {
		case "GetTable":
			tableDataFile = GetTable(P, qe.DB, steps)
		case "GetAllColumns":
			GetAllColumns(tableDataFile, &query)
		case "FilterByColumns":
			FilterByColumns(tableDataFile, &query, P)
		case "InsertRows":
			InsertRows(P, &query, qe.DB, tableDataFile)
		case "CreateTable":
			CreateTable(P, &query, qe.DB)
		case "JoinQueryTable":
			JoinTables(&query, tableDataFile, P.Joins[0].Condition)
		}
	}

	return query, nil
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

func GetTable(parsedQuery *ParsedQuery, bpm *storage.BufferPoolManager, step QueryStep) *os.File {
	manager := bpm.DiskScheduler.DiskManager
	tableNAME := parsedQuery.TableReferences[step.index]

	var tableObj *storage.TableObj
	var err error
	tableObj, found := manager.TableObjs[storage.TableName(tableNAME)]
	if !found {
		tableObj, err = manager.InMemoryTableSetUp(storage.TableName(tableNAME))
		if err != nil {
			fmt.Println(err)
			return nil
		}
	}

	fmt.Println("GOT TABLE")
	return tableObj.DataFile
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
	err := manager.WritePage(*updatedPage, offset, tableObj)
	if err != nil {
		errWrap := fmt.Errorf("Query error writing EXISTING page to data file: %w", err)
		fmt.Println(errWrap)
	}
}


func FindAvailablePage(tablePtr *os.File, parsedQuery *ParsedQuery, rows *storage.Row) *storage.Page {
	offset := 0
	page := storage.Page{Rows: make(map[uint64]storage.Row)}

	for {
		pageBytes := make([]byte, storage.PageSize)
		_, err := tablePtr.ReadAt(pageBytes, int64(offset))
		if err != nil {
			if err == io.EOF {
				fmt.Println("FindAvailablePage: End of file reached, creating new page")
				CreatePage(&page, rows, parsedQuery.TableReferences[0])
				cleanSize := getSizeOfIDAndRows(&page)
				fmt.Println(cleanSize, "=> end of file initial size")
				return &page
			}
			fmt.Printf("FindAvailablePage: Query error reading a page: %v\n", err)
			return nil
		}

		offset += storage.PageSize
		storage.DecodeV2(pageBytes, &page)

		cleanSize := getSizeOfIDAndRows(&page)

		if storage.PageSize > cleanSize {
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

func JoinTables(query *Query, filePtr *os.File, condition string) {

}

func FilterByColumns(filePtr *os.File, query *Query, P *ParsedQuery) {

}

func GetAllColumns(filePtr *os.File, query *Query) {

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
