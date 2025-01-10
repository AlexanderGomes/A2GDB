package tests

import (
	"a2gdb/cmd"
	"a2gdb/query-engine/engine"
	"a2gdb/storage-engine/storage"
	"a2gdb/util"
	"fmt"
	"log"
	"os"
	"strconv"
	"testing"

	"github.com/scylladb/go-set/strset"
)

var sharedDB *engine.QueryEngine

const REMOVE_DB = true
const REMOVE_LOGS = true

func TestMain(m *testing.M) {
	exitCode := m.Run()
	dbs := []string{
		"A2G_DB", "delete", "insert", "update", "wheres", "wheresRange", "wheresSorting", "wheresSortingLimit",
	}

	fmt.Println("Tearing down resources...")
	if REMOVE_DB {
		for _, db := range dbs {
			err := os.RemoveAll("./" + db)
			if err != nil {
				fmt.Printf("Error removing folder: %v\n", err)
			}
		}
	}

	if REMOVE_LOGS {
		err := os.RemoveAll("./app.json")
		if err != nil {
			log.Fatal(err)
		}
	}

	os.Exit(exitCode)
}

func TestInitDB(t *testing.T) {
	engine, err := cmd.InitDatabase(2, "./A2G_DB")
	if err != nil {
		t.Fatalf("Initializing DB failed: %s", err)
	}

	sharedDB = engine
}

func TestCreateTable(t *testing.T) {
	sql := "CREATE TABLE `User`(PRIMARY KEY(UserId), Username VARCHAR, Age INT, City VARCHAR)\n"
	encodedPlan1, err := util.SendSql(sql)
	if err != nil {
		t.Fatal("Error getting query plan: ", err)
	}

	sharedDB.EngineEntry(encodedPlan1)

	dbPath := "./A2G_DB"

	_, err = os.Stat(dbPath)
	if os.IsNotExist(err) {
		t.Fatal("Directory does not exist.")
	}
}

const expectedTupleNumber = 40
const tableName = "User"
const checkKey = "Username"
const checkVal = "JaneSmith"
const stressNumber = 1000
const expectedStressNumber = 1040

func TestInsert(t *testing.T) {
	t.Run("InsertMany", func(t *testing.T) {
		insertMany(t, expectedTupleNumber)
	})

	t.Run("CheckTupleNumber", func(t *testing.T) {
		checkTupleNumber(t, expectedTupleNumber)
	})

	t.Run("StressInsert", func(t *testing.T) {
		insertMany(t, stressNumber)
	})
	t.Run("checkTupleNumberStress", func(t *testing.T) {
		checkTupleNumber(t, expectedStressNumber)
	})

	t.Run("checkBp", func(t *testing.T) {
		checkBp(t)
	})
}

func checkBp(t *testing.T) {
	manager := sharedDB.BufferPoolManager.DiskManager
	tableObj, err := storage.GetTableObj(tableName, manager)
	if err != nil {
		t.Fatalf("couldn't get table object for table %s, error: %s", tableName, err)
	}

	tablePages, err := storage.GetTablePagesFromDiskTest(tableObj.DataFile)
	if err != nil {
		t.Fatalf("couldn't get table pages for table %s, error: %s", tableName, err)
	}

	for _, page := range tablePages {
		pageObj := tableObj.DirectoryPage.Value[storage.PageID(page.Header.ID)]
		for i := range pageObj.PointerArray {
			location := &pageObj.PointerArray[i]

			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				t.Fatalf("couldn't decode row, location: %+v, error: %s", location, err)
			}

			item := storage.Item{Key: row.ID}
			obj := tableObj.BpTree.Get(item)
			if obj == nil {
				t.Fatalf("couldn't find rowId: %d, for pageID: %d", row.ID, page.Header.ID)
			}
		}
	}

}

func TestUpdate(t *testing.T) {
	sql1 := "UPDATE `User` SET Age = 121209 WHERE Username = 'JaneSmith'\n"
	encodedPlan1, err := util.SendSql(sql1)
	if err != nil {
		t.Fatal(err)
	}
	sharedDB.EngineEntry(encodedPlan1)

	t.Run("Total Tuples After Update", func(t *testing.T) {
		checkTupleNumber(t, expectedStressNumber)
	})

	t.Run("Total Modified Tuples", func(t *testing.T) {
		checkModifiedTuples(t)
	})

	t.Run("CheckBp Atfer Update", func(t *testing.T) {
		checkBp(t)
	})

}

// func TestDelete(t *testing.T) {
// 	sql1 := fmt.Sprintf("DELETE FROM `%s` WHERE %s = '%s'\n", tableName, checkKey, checkVal)
// 	encodedPlan1, err := util.SendSql(sql1)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	sharedDB.EngineEntry(encodedPlan1)

// 	manager := sharedDB.BufferPoolManager.DiskManager
// 	tableObj, err := storage.GetTableObj(tableName, manager)
// 	if err != nil {
// 		t.Fatalf("couldn't get table object for table %s, error: %s", tableName, err)
// 	}

// 	tablePages, err := GetTablePagesFromDiskTest(tableObj.DataFile)
// 	if err != nil {
// 		t.Fatalf("couldn't get table pages for table %s, error: %s", tableName, err)
// 	}

// 	for _, page := range tablePages {
// 		pageObj, ok := tableObj.DirectoryPage.Value[storage.PageID(page.Header.ID)]
// 		if !ok {
// 			t.Fatalf("directory page contains wrong value for page: %+v", page)
// 		}

// 		for i := range pageObj.PointerArray {
// 			location := &pageObj.PointerArray[i]
// 			if !location.Free {
// 				t.Fatalf("location not marked as free when it should be: %+v", location)
// 			}

// 			if pageObj.ExactFreeMem != 0 {
// 				t.Fatalf("exact memory not zeroed, page %+v", page)
// 			}

// 			if pageObj.Level != engine.EMPTY_PAGE {
// 				t.Fatalf("not on expected level, page %+v", page)
// 			}
// 		}
// 	}
// }

// func TestCheckBpAfterDelete(t *testing.T) {
// 	rows, err := storage.GetAllRows(tableName, sharedDB.BufferPoolManager.DiskManager)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	if len(rows) != 0 {
// 		t.Fatal("not all rows were deleted data file")
// 	}

// 	manager := sharedDB.BufferPoolManager.DiskManager
// 	tableObj, err := storage.GetTableObj(tableName, manager)
// 	if err != nil {
// 		t.Fatalf("couldn't get table object for table %s, error: %s", tableName, err)
// 	}

// 	items := storage.GetAllItems(tableObj.BpTree)
// 	if len(items) != 0 {
// 		t.Fatal("not all rows were deleted from bp")
// 	}

// }

// func TestInsertAfterDelete(t *testing.T) {
// 	t.Run("insertManyAfterDelete", func(t *testing.T) {
// 		insertMany(t, expectedStressNumber)
// 	})

// 	t.Run("checkTupleNumber", func(t *testing.T) {
// 		checkTupleNumber(t, expectedStressNumber)
// 	})
// }

func TestSelects(t *testing.T) {
	t.Run("SelectStar", func(t *testing.T) {
		selectStart(t)
	})
	t.Run("selectFilter", func(t *testing.T) {
		selectFilter(t)
	})

	t.Run("selectWhere", func(t *testing.T) {
		selectWhere(t)
	})

	t.Run("selectWhereAnd", func(t *testing.T) {
		selectWhereAnd(t)
	})

	t.Run("FindPrimary", func(t *testing.T) {
		findByPrimary(t)
	})
}

const smallest = 1
const biggest = 1040

const ASC_LIMIT_1 = "SELECT Username, Age, City FROM `User` ORDER BY Age ASC LIMIT 1\n"
const DESC_LIMIT_1 = "SELECT Username, Age, City FROM `User` ORDER BY Age DESC LIMIT 1\n"
const ASC = "SELECT Username, Age, City FROM `User` ORDER BY Age ASC\n"
const DESC = "SELECT Username, Age, City FROM `User` ORDER BY Age DESC\n"

func TestOrderBy(t *testing.T) {
	queryMap := map[string]string{
		"ASC_LIMIT_1":  ASC_LIMIT_1,
		"DESC_LIMIT_1": DESC_LIMIT_1,
		"ASC":          ASC,
		"DESC":         DESC,
	}
	compKey := "Age"

	expectedColumns := strset.New("Username", "Age", "City")
	for identity, query := range queryMap {
		encodedPlan1, err := util.SendSql(query)
		if err != nil {
			t.Fatal(err)
		}

		rows, _, _ := sharedDB.EngineEntry(encodedPlan1)
		for _, row := range rows {
			if len(row.Values) != expectedColumns.Size() {
				t.Fatalf("incorrect number of columns returned")
			}

			for key := range row.Values {
				if !expectedColumns.Has(key) {
					t.Fatal("incorrect columns present")
				}
			}

			age, err := strconv.ParseInt(row.Values[compKey], 10, 64)
			if err != nil {
				t.Fatal(err)
			}

			last := rows[len(rows)-1]
			lastAge, err := strconv.ParseInt(last.Values[compKey], 10, 64)
			if err != nil {
				t.Fatal(err)
			}

			validateResults(t, identity, len(rows), int(age), int(lastAge), smallest, biggest, expectedStressNumber)
		}
	}
}

const COUNT = "SELECT City, COUNT(*) AS UserCount FROM `User` GROUP BY City\n"
const MAX = " SELECT City, MAX(Age) AS max_age FROM `User` GROUP BY City\n"
const MIN = "SELECT City, MIN(Age) AS max_age FROM `User` GROUP BY City\n"
const AVG = "SELECT City, AVG(Age) AS max_age FROM `User` GROUP BY City \n"
const SUM = "SELECT City, SUM(Age) AS max_age FROM `User` GROUP BY City\n"
const AVG_EXPECTED = 520
const SUM_EXPECTED = 541320

func TestGroupBy(t *testing.T) {
	queryMap := map[string]string{
		"COUNT": COUNT,
		"MAX":   MAX,
		"MIN":   MIN,
		"AVG":   AVG,
		"SUM":   SUM,
	}

	for identity, query := range queryMap {
		encodedPlan1, err := util.SendSql(query)
		if err != nil {
			t.Fatal(err)
		}

		expectedCity := "Los Angeles"
		_, groupMap, _ := sharedDB.EngineEntry(encodedPlan1)
		for k, v := range groupMap {
			if k != expectedCity {
				t.Fatalf("expected city: %s, received city: %s", expectedCity, k)
			}

			switch identity {
			case "COUNT":
				if v != expectedStressNumber {
					t.Fatalf("expected count: %d, received count: %d", expectedStressNumber, v)
				}
			case "MAX":
				if v != biggest {
					t.Fatalf("expected count: %d, received count: %d", biggest, v)
				}
			case "MIN":
				if v != smallest {
					t.Fatalf("expected count: %d, received count: %d", smallest, v)
				}
			case "AVG":
				if v != AVG_EXPECTED {
					t.Fatalf("expected count: %d, received count: %d", AVG_EXPECTED, v)
				}
			case "SUM":
				if v != SUM_EXPECTED {
					t.Fatalf("expected count: %d, received count: %d", SUM_EXPECTED, v)
				}
			}
		}
	}
}

const filterField = "Username"
const filterValue = "JaneSmith"
const modifiedField = "Age"
const modifiedValue = "121209"

func checkModifiedTuples(t *testing.T) {
	var modifiedCount int
	rows := getRows(t)
	for _, row := range rows {
		if row.Values[filterField] == filterValue && row.Values[modifiedField] == modifiedValue {
			modifiedCount++
		}
	}

	if modifiedCount != expectedStressNumber {
		t.Fatalf("expected count: [%d], modified count: [%d]", expectedStressNumber, modifiedCount)
	}
}

func checkUnmodifiedTuples(t *testing.T) {
	var unModifiedCount int
	rows := getRows(t)
	for _, row := range rows {
		if row.Values[modifiedField] != modifiedValue {
			unModifiedCount++
		}
	}

	if unModifiedCount != (EXPECTED_AFTER_UPDATE - expectedStressNumber) {
		t.Fatalf("expected count: [%d], modified count: [%d]", EXPECTED_AFTER_UPDATE-expectedStressNumber, unModifiedCount)
	}
}

const INSERT_AFTER_UPDATE = expectedStressNumber + 1000
const EXPECTED_AFTER_UPDATE = expectedStressNumber + expectedStressNumber + 1000

func TestInsertAfterUpdate(t *testing.T) {
	t.Run("StressInsert", func(t *testing.T) {
		insertMany(t, INSERT_AFTER_UPDATE)
	})

	t.Run("CheckTupleNumber", func(t *testing.T) {
		checkTupleNumber(t, EXPECTED_AFTER_UPDATE)
	})

	t.Run("Total Modified Tuples", func(t *testing.T) {
		checkModifiedTuples(t)
	})

	t.Run("Total Unmodified Tuples", func(t *testing.T) {
		checkUnmodifiedTuples(t)
	})
}

func checkTupleNumber(t *testing.T, expectedNumber int) {
	var count int
	manager := sharedDB.BufferPoolManager.DiskManager
	tableObj, err := storage.GetTableObj(tableName, manager)
	if err != nil {
		t.Fatalf("couldn't get table object for table %s, error: %s", tableName, err)
	}

	tablePages, err := storage.GetTablePagesFromDiskTest(tableObj.DataFile)
	if err != nil {
		t.Fatalf("couldn't get table pages for table %s, error: %s", tableName, err)
	}

	var innerCount int
	for _, page := range tablePages {
		pageObj := tableObj.DirectoryPage.Value[storage.PageID(page.Header.ID)]
		for i := range pageObj.PointerArray {
			location := &pageObj.PointerArray[i]

			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				t.Fatalf("couldn't decode row, location: %+v, error: %s", location, err)
			}

			innerCount++
			if row.Values[checkKey] == checkVal {
				count++
			}
		}
	}

	if count != expectedNumber {
		t.Fatalf("expected count: [%d], actual count: [%d]", expectedNumber, count)
	}
}

func insertMany(t *testing.T, x int) {
	for i := range x {
		sql1 := fmt.Sprintf("INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', %d, 'Los Angeles')\n", i+1)
		encodedPlan1, err := util.SendSql(sql1)
		if err != nil {
			t.Fatal(err)
		}
		sharedDB.EngineEntry(encodedPlan1)
	}
}

func selectFilter(t *testing.T) {
	expectedColumns := strset.New("Username", "Age")

	sql1 := fmt.Sprintf("SELECT Username, Age FROM `%s`\n", tableName)
	encodedPlan1, err := util.SendSql(sql1)
	if err != nil {
		t.Fatal(err)
	}

	rows, _, _ := sharedDB.EngineEntry(encodedPlan1)
	if len(rows) != expectedStressNumber {
		t.Fatalf("incorrect number of rows returned")
	}

	for _, row := range rows {
		if len(row.Values) != expectedColumns.Size() {
			t.Fatalf("incorrect number of columns returned")
		}

		for key := range row.Values {
			if !expectedColumns.Has(key) {
				t.Fatal("incorrect columns present")
			}
		}
	}
}

func selectStart(t *testing.T) *storage.RowV2 {
	sql1 := fmt.Sprintf("SELECT * FROM `%s`\n", tableName)
	encodedPlan1, err := util.SendSql(sql1)
	if err != nil {
		t.Fatal(err)
	}

	rows, _, _ := sharedDB.EngineEntry(encodedPlan1)
	if len(rows) != expectedStressNumber {
		t.Fatalf("incorrect number of rows returned")
	}

	tableInfo := sharedDB.BufferPoolManager.DiskManager.PageCatalog.Tables[tableName]
	if len(rows[4].Values) != len(tableInfo.Schema) {
		t.Fatalf("wrong number of columns returned")
	}

	return rows[0]
}

func selectWhere(t *testing.T) {
	expectedColumns := strset.New("Username", "Age", "City")
	compKey := "Age"
	conditions := []string{">", "=", "<"}
	compVal := 20

	for _, condition := range conditions {
		sql1 := fmt.Sprintf("SELECT Username, Age, City FROM `%s` WHERE Age %s 20\n", tableName, condition)
		encodedPlan1, err := util.SendSql(sql1)
		if err != nil {
			t.Fatal(err)
		}

		rows, _, _ := sharedDB.EngineEntry(encodedPlan1)
		for _, row := range rows {
			if len(row.Values) != expectedColumns.Size() {
				t.Fatalf("incorrect number of columns returned")
			}

			for key := range row.Values {
				if !expectedColumns.Has(key) {
					t.Fatal("incorrect columns present")
				}
			}

			age, err := strconv.Atoi(row.Values[compKey])
			if err != nil {
				t.Fatal(err)
			}

			switch condition {
			case ">":
				if age < compVal {
					t.Fatal("incorrect filter value passed (BIGGER_THAN)")
				}
			case "=":
				if age != compVal {
					t.Fatal("incorrect filter value passed (EQUALS)")
				}
			case "<":
				if age > compVal {
					t.Fatal("incorrect filter value passed (BIGGER_THAN)")
				}
			}
		}
	}
}

func selectWhereAnd(t *testing.T) {
	expectedColumns := strset.New("Username", "Age", "City")
	compKey := "Age"
	compValLeft := 20
	compValRight := 30

	sql1 := "SELECT Username, Age, City FROM `User` WHERE Age BETWEEN 20 AND 30\n"
	encodedPlan1, err := util.SendSql(sql1)
	if err != nil {
		t.Fatal(err)
	}

	rows, _, _ := sharedDB.EngineEntry(encodedPlan1)

	for _, row := range rows {
		if len(row.Values) != expectedColumns.Size() {
			t.Fatalf("incorrect number of columns returned")
		}

		for key := range row.Values {
			if !expectedColumns.Has(key) {
				t.Fatal("incorrect columns present")
			}
		}

		age, err := strconv.Atoi(row.Values[compKey])
		if err != nil {
			t.Fatal(err)
		}

		if age < compValLeft || age > compValRight {
			t.Fatalf("row: [%+v] contains incorrect result", row)
		}
	}
}

func findByPrimary(t *testing.T) {
	row := selectStart(t)

	sql1 := fmt.Sprintf("SELECT * FROM `%s` WHERE UserId = CAST('%d' AS DECIMAL(20,0))\n", tableName, row.ID)
	encodedPlan1, err := util.SendSql(sql1)
	if err != nil {
		t.Fatal(err)
	}

	rows, _, _ := sharedDB.EngineEntry(encodedPlan1)
	user := rows[0]
	if len(rows) != 1 {
		t.Fatalf("Returned %d users instead of one", len(rows))
	}

	if user.ID != row.ID {
		t.Fatal("wrong user")
	}

	tableInfo := sharedDB.BufferPoolManager.DiskManager.PageCatalog.Tables[tableName]
	if len(rows[0].Values) != len(tableInfo.Schema) {
		t.Fatalf("wrong number of columns returned")
	}
}

func validateResults(t *testing.T, identity string, rowLength, age, lastAge, smallest, biggest, expectedStressNumber int) {
	switch identity {
	case "ASC_LIMIT_1":
		checkRowCount(t, identity, rowLength, 1)
		checkAge(t, identity, age, smallest)

	case "DESC_LIMIT_1":
		checkRowCount(t, identity, rowLength, 1)
		checkAge(t, identity, age, biggest)

	case "ASC":
		checkRowCount(t, identity, rowLength, expectedStressNumber)
		checkOrder(t, identity, lastAge, biggest)
	case "DESC":
		checkRowCount(t, identity, rowLength, expectedStressNumber)
		checkOrder(t, identity, lastAge, smallest)
	}
}

func checkRowCount(t *testing.T, identity string, rowLength, expectedCount int) {
	if rowLength != expectedCount {
		t.Fatalf("[%s] incorrect number of rows returned: %d (expected: %d)", identity, rowLength, expectedCount)
	}
}

func checkAge(t *testing.T, identity string, actual, expected int) {
	if actual != expected {
		t.Fatalf("[%s] wrong age returned: %d (expected: %d)", identity, actual, expected)
	}
}

func checkOrder(t *testing.T, identity string, actual, expected int) {
	if actual != expected {
		t.Fatalf("[%s] wrong order: %d (expected: %d)", identity, actual, expected)
	}
}

func getRows(t *testing.T) []*storage.RowV2 {
	var rows []*storage.RowV2

	manager := sharedDB.BufferPoolManager.DiskManager
	tableObj, err := storage.GetTableObj(tableName, manager)
	if err != nil {
		t.Fatalf("couldn't get table object for table %s, error: %s", tableName, err)
	}

	tablePages, err := storage.GetTablePagesFromDiskTest(tableObj.DataFile)
	if err != nil {
		t.Fatalf("couldn't get table pages for table %s, error: %s", tableName, err)
	}

	for _, page := range tablePages {
		pageObj, ok := tableObj.DirectoryPage.Value[storage.PageID(page.Header.ID)]
		if !ok {
			t.Fatalf("directory page contains wrong value for page: %+v", page)
		}

		for _, location := range pageObj.PointerArray {
			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				t.Fatalf("couldn't decode row, location: %+v, error: %s", location, err)
			}

			rows = append(rows, row)
		}
	}

	return rows
}
