package tests

import (
	"a2gdb/cmd"
	"a2gdb/util"
	"fmt"
	"log"
	"os"
	"strconv"
	"testing"

	"github.com/scylladb/go-set/strset"
)

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

		rows, _, res := sharedDB.EngineEntry(encodedPlan1)
		if res.Error != nil {
			t.Fatal(res.Error)
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

		first := rows[0]
		firstAge, err := strconv.ParseInt(first.Values[compKey], 10, 64)
		if err != nil {
			t.Fatal(err)
		}

		last := rows[len(rows)-1]
		lastAge, err := strconv.ParseInt(last.Values[compKey], 10, 64)
		if err != nil {
			t.Fatal(err)
		}

		validateResults(t, identity, len(rows), int(firstAge), int(lastAge), smallest, biggest, expectedStressNumber)
	}
}

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

func TestUpdate(t *testing.T) {
	sql1 := fmt.Sprintf("UPDATE `User` SET %s = %s WHERE Username = 'JaneSmith'\n", modifiedField, modifiedValue)
	encodedPlan1, err := util.SendSql(sql1)
	if err != nil {
		t.Fatal(err)
	}
	_, _, res := sharedDB.EngineEntry(encodedPlan1)

	if res.Error != nil {
		t.Fatal(res.Error)
	}

	t.Run("Total Tuples After Update", func(t *testing.T) {
		checkTupleNumber(t, expectedStressNumber)
	})

	t.Run("Total Modified Tuples", func(t *testing.T) {
		checkModifiedTuples(t)
	})

}

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

// func TestInsertAfterDelete(t *testing.T) {
// 	t.Run("insertManyAfterDelete", func(t *testing.T) {
// 		insertMany(t, expectedStressNumber)
// 	})

// 	t.Run("checkTupleNumber", func(t *testing.T) {
// 		checkTupleNumber(t, expectedStressNumber)
// 	})
// }
