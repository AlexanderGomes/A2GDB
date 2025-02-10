package tests

// import (
// 	"a2gdb/engine"
// 	"fmt"
// 	"strconv"
// 	"testing"

// 	"github.com/scylladb/go-set/strset"
// )

// var sharedDB *engine.QueryEngine

// const filterField = "Username"
// const filterValue = "JaneSmith"
// const modifiedField = "Age"
// const modifiedValue = "189222"

// const expectedTupleNumber = 40
// const tableName = "User"
// const checkKey = "Username"
// const checkVal = "JaneSmith"
// const stressNumber = 1000
// const expectedStressNumber = 1040

// const INSERT_AFTER_UPDATE = expectedStressNumber + 1000
// const EXPECTED_AFTER_UPDATE = expectedStressNumber + expectedStressNumber + 1000

// const smallest = 1
// const biggest = 1000

// const ASC_LIMIT_1 = "SELECT Username, Age, City FROM `User` ORDER BY Age ASC LIMIT 1\n"
// const DESC_LIMIT_1 = "SELECT Username, Age, City FROM `User` ORDER BY Age DESC LIMIT 1\n"
// const ASC = "SELECT Username, Age, City FROM `User` ORDER BY Age ASC\n"
// const DESC = "SELECT Username, Age, City FROM `User` ORDER BY Age DESC\n"

// const COUNT = "SELECT City, COUNT(*) AS UserCount FROM `User` GROUP BY City\n"
// const MAX = " SELECT City, MAX(Age) AS max_age FROM `User` GROUP BY City\n"
// const MIN = "SELECT City, MIN(Age) AS max_age FROM `User` GROUP BY City\n"
// const AVG = "SELECT City, AVG(Age) AS max_age FROM `User` GROUP BY City \n"
// const SUM = "SELECT City, SUM(Age) AS max_age FROM `User` GROUP BY City\n"
// const AVG_EXPECTED = 482
// const SUM_EXPECTED = 501320

// func checkUnmodifiedTuples(t *testing.T) {
// 	var unModifiedCount int
// 	rows := getRows(t)
// 	for _, row := range rows {
// 		if row.Values[modifiedField] != modifiedValue {
// 			unModifiedCount++
// 		}
// 	}

// 	if unModifiedCount != (EXPECTED_AFTER_UPDATE - expectedStressNumber) {
// 		t.Fatalf("expected count: [%d], modified count: [%d]", EXPECTED_AFTER_UPDATE-expectedStressNumber, unModifiedCount)
// 	}
// }

// func checkTupleNumber(t *testing.T, expectedNumber int) {
// 	var count int
// 	manager := sharedDB.BufferPoolManager.DiskManager
// 	tableObj, err := engine.GetTableObj(tableName, manager)
// 	if err != nil {
// 		t.Fatalf("couldn't get table object for table %s, error: %s", tableName, err)
// 	}

// 	tablePages, err := engine.GetTablePagesFromDiskTest(tableObj.DataFile)
// 	if err != nil {
// 		t.Fatalf("couldn't get table pages for table %s, error: %s", tableName, err)
// 	}

// 	var innerCount int
// 	for _, page := range tablePages {
// 		pageObj := tableObj.DirectoryPage.Value[engine.PageID(page.Header.ID)]
// 		for i := range pageObj.PointerArray {
// 			location := &pageObj.PointerArray[i]

// 			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
// 			row, err := engine.DecodeRow(rowBytes)
// 			if err != nil {
// 				t.Fatalf("couldn't decode row, location: %+v, error: %s", location, err)
// 			}

// 			innerCount++
// 			if row.Values[checkKey] == checkVal {
// 				count++
// 			}
// 		}
// 	}

// 	if count != expectedNumber {
// 		t.Fatalf("expected count: [%d], matched rows count: [%d], total rows count: [%d]", expectedNumber, count, innerCount)
// 	}
// }

// func insertMany(t *testing.T, x int) {
// 	for i := range x {
// 		sql1 := fmt.Sprintf("INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', %d, 'Los Angeles')\n", i+1)
// 		encodedPlan1, err := engine.SendSql(sql1)
// 		if err != nil {
// 			t.Fatal(err)
// 		}
// 		_, _, res := sharedDB.EngineEntry(encodedPlan1, false)
// 		if res.Error != nil {
// 			t.Fatal("InsertMany Failed: ", res.Error)
// 		}
// 	}
// }

// func selectFilter(t *testing.T) {
// 	expectedColumns := strset.New("Username", "Age")

// 	sql1 := fmt.Sprintf("SELECT Username, Age FROM `%s`\n", tableName)
// 	encodedPlan1, err := engine.SendSql(sql1)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	rows, _, _ := sharedDB.EngineEntry(encodedPlan1, false)
// 	if len(rows) != expectedStressNumber {
// 		t.Fatalf("incorrect number of rows returned")
// 	}

// 	for _, row := range rows {
// 		if len(row.Values) != expectedColumns.Size() {
// 			t.Fatalf("incorrect number of columns returned")
// 		}

// 		for key := range row.Values {
// 			if !expectedColumns.Has(key) {
// 				t.Fatal("incorrect columns present")
// 			}
// 		}
// 	}
// }

// func selectStart(t *testing.T) *engine.RowV2 {
// 	sql1 := fmt.Sprintf("SELECT * FROM `%s`\n", tableName)
// 	encodedPlan1, err := engine.SendSql(sql1)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	rows, _, _ := sharedDB.EngineEntry(encodedPlan1, false)
// 	if len(rows) != expectedStressNumber {
// 		t.Fatalf("incorrect number of rows returned")
// 	}

// 	tableInfo := sharedDB.BufferPoolManager.DiskManager.PageCatalog.Tables[tableName]
// 	if len(rows[4].Values) != len(tableInfo.Schema) {
// 		t.Fatalf("wrong number of columns returned")
// 	}

// 	return rows[0]
// }

// func selectWhere(t *testing.T) {
// 	expectedColumns := strset.New("Username", "Age", "City")
// 	compKey := "Age"
// 	conditions := []string{">", "=", "<"}
// 	compVal := 20

// 	for _, condition := range conditions {
// 		sql1 := fmt.Sprintf("SELECT Username, Age, City FROM `%s` WHERE Age %s 20\n", tableName, condition)
// 		encodedPlan1, err := engine.SendSql(sql1)
// 		if err != nil {
// 			t.Fatal(err)
// 		}

// 		rows, _, _ := sharedDB.EngineEntry(encodedPlan1, false)
// 		for _, row := range rows {
// 			if len(row.Values) != expectedColumns.Size() {
// 				t.Fatalf("incorrect number of columns returned")
// 			}

// 			for key := range row.Values {
// 				if !expectedColumns.Has(key) {
// 					t.Fatal("incorrect columns present")
// 				}
// 			}

// 			age, err := strconv.Atoi(row.Values[compKey])
// 			if err != nil {
// 				t.Fatal(err)
// 			}

// 			switch condition {
// 			case ">":
// 				if age < compVal {
// 					t.Fatal("incorrect filter value passed (BIGGER_THAN)")
// 				}
// 			case "=":
// 				if age != compVal {
// 					t.Fatal("incorrect filter value passed (EQUALS)")
// 				}
// 			case "<":
// 				if age > compVal {
// 					t.Fatal("incorrect filter value passed (BIGGER_THAN)")
// 				}
// 			}
// 		}
// 	}
// }

// func selectWhereAnd(t *testing.T) {
// 	expectedColumns := strset.New("Username", "Age", "City")
// 	compKey := "Age"
// 	compValLeft := 20
// 	compValRight := 30

// 	sql1 := "SELECT Username, Age, City FROM `User` WHERE Age BETWEEN 20 AND 30\n"
// 	encodedPlan1, err := engine.SendSql(sql1)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	rows, _, _ := sharedDB.EngineEntry(encodedPlan1, false)

// 	for _, row := range rows {
// 		if len(row.Values) != expectedColumns.Size() {
// 			t.Fatalf("incorrect number of columns returned")
// 		}

// 		for key := range row.Values {
// 			if !expectedColumns.Has(key) {
// 				t.Fatal("incorrect columns present")
// 			}
// 		}

// 		age, err := strconv.Atoi(row.Values[compKey])
// 		if err != nil {
// 			t.Fatal(err)
// 		}

// 		if age < compValLeft || age > compValRight {
// 			t.Fatalf("row: [%+v] contains incorrect result", row)
// 		}
// 	}
// }

// func findByPrimary(t *testing.T) {
// 	row := selectStart(t)

// 	sql1 := fmt.Sprintf("SELECT * FROM `%s` WHERE UserId = CAST('%d' AS DECIMAL(20,0))\n", tableName, row.ID)
// 	encodedPlan1, err := engine.SendSql(sql1)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	rows, _, _ := sharedDB.EngineEntry(encodedPlan1, false)
// 	user := rows[0]
// 	if len(rows) != 1 {
// 		t.Fatalf("Returned %d users instead of one", len(rows))
// 	}

// 	if user.ID != row.ID {
// 		t.Fatal("wrong user")
// 	}

// 	tableInfo := sharedDB.BufferPoolManager.DiskManager.PageCatalog.Tables[tableName]
// 	if len(rows[0].Values) != len(tableInfo.Schema) {
// 		t.Fatalf("wrong number of columns returned")
// 	}
// }

// func validateResults(t *testing.T, identity string, rowLength, age, lastAge, smallest, biggest, expectedStressNumber int) {
// 	switch identity {
// 	case "ASC_LIMIT_1":
// 		checkRowCount(t, identity, rowLength, 1)
// 		checkAge(t, identity, age, smallest)

// 	case "DESC_LIMIT_1":
// 		checkRowCount(t, identity, rowLength, 1)
// 		checkAge(t, identity, age, biggest)

// 	case "ASC":
// 		checkRowCount(t, identity, rowLength, expectedStressNumber)
// 		checkOrder(t, identity, lastAge, biggest)
// 	case "DESC":
// 		checkRowCount(t, identity, rowLength, expectedStressNumber)
// 		checkOrder(t, identity, lastAge, smallest)
// 	}
// }

// func checkRowCount(t *testing.T, identity string, rowLength, expectedCount int) {
// 	if rowLength != expectedCount {
// 		t.Fatalf("[%s] incorrect number of rows returned: %d (expected: %d)", identity, rowLength, expectedCount)
// 	}
// }

// func checkAge(t *testing.T, identity string, actual, expected int) {
// 	if actual != expected {
// 		t.Fatalf("[%s] wrong age returned: %d (expected: %d)", identity, actual, expected)
// 	}
// }

// func checkOrder(t *testing.T, identity string, actual, expected int) {
// 	if actual != expected {
// 		t.Fatalf("[%s] wrong order: %d (expected: %d)", identity, actual, expected)
// 	}
// }

// func getRows(t *testing.T) []*engine.RowV2 {
// 	var rows []*engine.RowV2

// 	manager := sharedDB.BufferPoolManager.DiskManager
// 	tableObj, err := engine.GetTableObj(tableName, manager)
// 	if err != nil {
// 		t.Fatalf("couldn't get table object for table %s, error: %s", tableName, err)
// 	}

// 	tablePages, err := engine.GetTablePagesFromDiskTest(tableObj.DataFile)
// 	if err != nil {
// 		t.Fatalf("couldn't get table pages for table %s, error: %s", tableName, err)
// 	}

// 	for _, page := range tablePages {
// 		pageObj, ok := tableObj.DirectoryPage.Value[engine.PageID(page.Header.ID)]
// 		if !ok {
// 			t.Fatalf("directory page contains wrong value for page: %+v", page)
// 		}

// 		for _, location := range pageObj.PointerArray {
// 			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
// 			row, err := engine.DecodeRow(rowBytes)
// 			if err != nil {
// 				t.Fatalf("couldn't decode row, location: %+v, error: %s", location, err)
// 			}

// 			rows = append(rows, row)
// 		}
// 	}

// 	return rows
// }

// func checkModifiedTuples(t *testing.T) {
// 	var modifiedCount int
// 	rows := getRows(t)
// 	for _, row := range rows {
// 		if row.Values[filterField] == filterValue && row.Values[modifiedField] == modifiedValue {
// 			modifiedCount++
// 		}
// 	}

// 	if modifiedCount != expectedStressNumber {
// 		t.Fatalf("expected count: [%d], modified count: [%d]", expectedStressNumber, modifiedCount)
// 	}
// }