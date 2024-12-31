package tests

import (
	"a2gdb/cmd"
	"a2gdb/query-engine/engine"
	"a2gdb/storage-engine/storage"
	"a2gdb/util"
	"fmt"
	"os"
	"testing"
)

var sharedDB *engine.QueryEngine

func TestMain(m *testing.M) {
	exitCode := m.Run()

	fmt.Println("Tearing down resources...")
	err := os.RemoveAll("./A2G_DB")
	if err != nil {
		fmt.Printf("Error removing folder: %v\n", err)
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

	dbPath := "./A2G_DB" // Replace with your directory path

	_, err = os.Stat(dbPath)
	if os.IsNotExist(err) {
		t.Fatal("Directory does not exist.")
	}
}

const expectedTupleNumber = 40
const tableName = "User"
const checkKey = "Username"
const checkVal = "JaneSmith"
const stressNumber = 10000
const expectedStressNumber = 10040

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

}

func checkTupleNumber(t *testing.T, expectedNumber int) {
	var count int
	tableObj, err := sharedDB.GetTable(tableName)
	if err != nil {
		t.Fatalf("couldn't get table object for table %s, error: %s", tableName, err)
	}

	tablePages, err := storage.GetTablePages(tableObj.DataFile, nil)
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

			if row.Values[checkKey] == checkVal {
				count++
			}
		}
	}

	fmt.Println(count)
	if count != expectedNumber {
		t.Fatal("Wrong number of tuples inserted")
	}
}

func insertMany(t *testing.T, x int) {
	for i := range x {
		sql1 := fmt.Sprintf("INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', %d, 'Los Angeles')\n", i)
		encodedPlan1, err := util.SendSql(sql1)
		if err != nil {
			t.Fatal(err)
		}
		sharedDB.EngineEntry(encodedPlan1)
	}
}
