package tests

import (
	"disk-db/cmd"
	queryengine "disk-db/query-engine"
	"disk-db/storage"
	"fmt"
	"testing"

	"github.com/google/btree"
)

const (
	DB_NAME         = "Testing_DB"
	TABLE_NAME      = "User"
	ROWS_NUM        = 1000
	ROWS_PER_INSERT = 20
	COLUMNS_NUM     = 2
)

func TestBaseQueries(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Test panicked: %v", r)
		}
	}()

	qry := setupDatabase(t)

	t.Run("Create Table", func(t *testing.T) {
		createTable(t, qry)
	})

	t.Run("Insert Query", func(t *testing.T) {
		t.Run("INSERT", func(t *testing.T) {
			insertRows(t, qry)
		})
		t.Run("Check database state", func(t *testing.T) {
			checkDb(t, qry)
		})
	})

	t.Run("Delete Query", func(t *testing.T) {
		t.Run("DELETE", func(t *testing.T) {
			deleteRows(t, qry)
		})

		t.Run("Check database state", func(t *testing.T) {
			checkDb(t, qry)
		})

		t.Run("Select After Delete", func(t *testing.T) {
			selects(t, qry)
		})
	})

	t.Run("Update Query", func(t *testing.T) {
		updateVals := [][2]string{
			{"'sander12'", "'small'"},
			{"'sander10'", "'small_medium_medium_medium'"},
			{"'sander19'", "'small_medium_medium_medium_small_medium_medium_medium_small_medium_medium_medium_small_medium_medium_medium'"},
		}
		t.Run("UPDATE", func(t *testing.T) {
			update(t, qry, updateVals)
		})

		t.Run("Check database state", func(t *testing.T) {
			checkDb(t, qry)
		})

		t.Run("Select After Update", func(t *testing.T) {
			selects(t, qry)
		})

		t.Run("Check Updated Values", func(t *testing.T) {
			CheckUpdatedVals(t, qry, updateVals)
		})
	})
}

func CheckUpdatedVals(t *testing.T, qry *queryengine.QueryEngine, updateVals [][2]string) {
	for _, inputs := range updateVals {
		new := inputs[1]
		selectQuery := fmt.Sprintf(`SELECT * FROM User Where Username = %s;`, new)
		res, err := qry.QueryEntryPoint(selectQuery)
		if err != nil {
			t.Fatalf("selecting new value after update failed: %v", err)
		}

		if len(res.Result) != ROWS_NUM {
			t.Fatal("CheckUpdatedVals: updated result mismatche")
		}

		old := inputs[0]
		oldSelectQuery := fmt.Sprintf(`SELECT * FROM User Where Username = %s;`, old)
		res, err = qry.QueryEntryPoint(oldSelectQuery)
		if err != nil {
			t.Fatalf("selecting new value after update failed: %v", err)
		}

		if len(res.Result) > 0 {
			t.Fatal("old tuple still on DB")
		}
	}
}

func update(t *testing.T, qry *queryengine.QueryEngine, updateVals [][2]string) {
	old := updateVals[0][0]
	new := updateVals[0][1]

	smallUpdate := fmt.Sprintf(`UPDATE User SET Username = %s WHERE Username = %s;`, new, old)
	_, err := qry.QueryEntryPoint(smallUpdate)
	if err != nil {
		t.Fatalf("update (small query): %v", err)
	}

	old = updateVals[1][0]
	new = updateVals[1][1]

	mediumUpdate := fmt.Sprintf(`UPDATE User SET Username = %s WHERE Username = %s;`, new, old)
	_, err = qry.QueryEntryPoint(mediumUpdate)
	if err != nil {
		t.Fatalf("update (medium query): %v", err)
	}

	old = updateVals[2][0]
	new = updateVals[2][1]

	bigUpdate := fmt.Sprintf(`UPDATE User SET Username = %s WHERE Username = %s;`, new, old)
	_, err = qry.QueryEntryPoint(bigUpdate)
	if err != nil {
		t.Fatalf("update (big query): %v", err)
	}
}

func selects(t *testing.T, qry *queryengine.QueryEngine) {
	tableInfo, ok := qry.Disk.PageCatalog.Tables[TABLE_NAME]
	if !ok {
		t.Fatal("selects: table info not found")
	}

	selectStarQuery := `SELECT * FROM User Where Username = 'sander15';`
	res, err := qry.QueryEntryPoint(selectStarQuery)
	if err != nil {
		t.Fatalf("selecting * by username failed: %v", err)
	}
	checkSelectStarWhere(t, res, tableInfo)

	selectColumnQuery := `SELECT Username, UserID FROM User Where Username = 'sander15';`
	res, err = qry.QueryEntryPoint(selectColumnQuery)
	if err != nil {
		t.Fatalf("selecting column with where clause failed: %v", err)
	}
	checkSelectColumnsWhere(t, res)

	selectStar := `SELECT * FROM User;`
	res, err = qry.QueryEntryPoint(selectStar)
	if err != nil {
		t.Fatalf("selecting * failed: %v", err)
	}
	checkSelectStarNoWhere(t, res, tableInfo)
}

func checkSelectStarNoWhere(t *testing.T, res queryengine.Query, tableInfo storage.TableInfo) {
	rows_inserted_minus_rows_deleted := ROWS_NUM*ROWS_PER_INSERT - ROWS_NUM
	if len(res.Result) != rows_inserted_minus_rows_deleted {
		t.Fatal("selects: wrong number of rows inserted")
	}

	var numBrokenRows int
	for _, row := range res.Result {
		for key := range tableInfo.Schema {
			if _, ok := row.Values[key]; !ok {
				numBrokenRows++
				break
			}
		}
	}

	if numBrokenRows > 0 {
		fmt.Println("number of broken rows: ", numBrokenRows)
		t.Fatal("selects (select start): Schema inconsistency")
	}
}

func checkSelectColumnsWhere(t *testing.T, res queryengine.Query) {
	if len(res.Result) != ROWS_NUM {
		t.Fatal("selects: less rows then expected")
	}

	for _, row := range res.Result {
		target := "'sander15'"
		foundUsername := row.Values["Username"]
		if foundUsername != target {
			t.Fatal("selects: different Usernames")
		}

		if len(row.Values) != COLUMNS_NUM {
			t.Fatal("selects: wrong number of columns")
		}
	}
}

func checkSelectStarWhere(t *testing.T, res queryengine.Query, tableInfo storage.TableInfo) {
	if len(res.Result) != ROWS_NUM {
		t.Fatal("selects: less rows then expected")
	}

	for _, row := range res.Result {
		for key := range tableInfo.Schema {
			if _, ok := row.Values[key]; !ok {
				t.Fatal("selects (check star where clause): Schema inconsistency")
			}

			target := "'sander15'"
			username := row.Values["Username"]
			if username != target {
				t.Fatal("selects: different usernames")
			}
		}
	}
}

func deleteRows(t *testing.T, qry *queryengine.QueryEngine) {
	deleteTableQuery := `DELETE FROM User Where Username = 'sander1';`
	_, err := qry.QueryEntryPoint(deleteTableQuery)
	if err != nil {
		t.Fatalf("deleting from table failed: %v", err)
	}
}

func setupDatabase(t *testing.T) *queryengine.QueryEngine {
	qry, err := cmd.InitDatabase(3, DB_NAME)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	if qry == nil {
		t.Fatal("Expected non-nil query object")
	}
	return qry
}

func createTable(t *testing.T, qry *queryengine.QueryEngine) {
	createTableQuery := `
        CREATE TABLE User (
            UserID INT PRIMARY KEY,
            Username VARCHAR,
            Age INT,
            City VARCHAR
        );`
	_, err := qry.QueryEntryPoint(createTableQuery)
	if err != nil {
		t.Fatalf("Creating table failed: %v", err)
	}
}

func checkDb(t *testing.T, qry *queryengine.QueryEngine) {
	tableObj, ok := qry.Disk.TableObjs[TABLE_NAME]
	if !ok {
		t.Fatal("tableObj not found")
	}

	pages, err := storage.GetTablePages(tableObj.DataFile, nil)
	if err != nil {
		t.Fatalf("couldn't get pages: %v", err)
	}

	if len(pages) == 0 {
		t.Fatalf("page slice is empty: %v", err)
	}

	_, dirDiskPage, err := storage.GetDirInfo(DB_NAME, TABLE_NAME)
	if err != nil {
		t.Fatalf("couldn't get dir info from disk: %v", err)
	}

	treeDisk, _, err := storage.GetBpTree(DB_NAME, TABLE_NAME)
	if err != nil {
		t.Fatalf("couldn't get bptree info from disk: %v", err)
	}

	for _, page := range pages {
		InMemoryCheck(t, tableObj, page)
		FromDiskCheck(t, dirDiskPage, page, treeDisk)
	}

}

func CheckLocation(t *testing.T, pageObj *storage.PageInfo, page *storage.PageV2, tree *btree.BTree, testLocation string) {
	for _, location := range pageObj.PointerArray {
		if !location.Free {
			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				t.Fatalf("CheckLocation : failed to decode row: %v", err)
			}

			rowId := row.ID
			rowOffset := pageObj.Offset

			item := storage.Item{
				Key:   rowId,
				Value: rowOffset,
			}

			indexItem := tree.Get(item)

			if indexItem == nil {
				t.Fatalf("CheckLocation (%v): bptree outdated", testLocation)
			}
		}
	}
}

func FromDiskCheck(t *testing.T, dirDiskPage *storage.DirectoryPageV2, page *storage.PageV2, treeDisk *btree.BTree) {
	pageObj, ok := dirDiskPage.Value[storage.PageID(page.Header.ID)]
	if !ok {
		t.Fatal("directory page from disk is not updated")
	}

	CheckLocation(t, pageObj, page, treeDisk, "disk")
}

func InMemoryCheck(t *testing.T, tableObj *storage.TableObj, page *storage.PageV2) {
	pageObj, ok := tableObj.DirectoryPage.Value[storage.PageID(page.Header.ID)]
	if !ok {
		t.Fatal("directory page is not updated")
	}

	CheckLocation(t, pageObj, page, tableObj.BpTree, "memory")
}

func insertRows(t *testing.T, qry *queryengine.QueryEngine) {
	for i := 0; i < ROWS_NUM; i++ {
		_, err := qry.QueryEntryPoint(`INSERT INTO User (Username, Age, City) VALUES
		('sander1', 12, 'richmond'),
		('sander2', 15, 'richmond'),
		('sander3', 23, 'richmond'),
		('sander4', 11, 'richmond'),
		('sander5', 7, 'richmond');`)
		if err != nil {
			t.Fatalf("Inserting rows failed: %v", err)
		}

		_, err = qry.QueryEntryPoint(`INSERT INTO User (Username, Age, City) VALUES
		('sander6', 58, 'san pablo'),
		('sander7', 77, 'san pablo'),
		('sander8', 31, 'san pablo'),
		('sander9', 21, 'san pablo'),
		('sander10', 93, 'san pablo');`)
		if err != nil {
			t.Fatalf("Inserting rows failed: %v", err)
		}

		_, err = qry.QueryEntryPoint(`INSERT INTO User (Username, Age, City) VALUES
		('sander11', 16, 'pinole'),
		('sander12', 25, 'pinole'),
		('sander13', 11, 'pinole'),
		('sander14', 12, 'pinole'),
		('sander15', 10, 'pinole');`)
		if err != nil {
			t.Fatalf("Inserting rows failed: %v", err)
		}

		_, err = qry.QueryEntryPoint(`INSERT INTO User (Username, Age, City) VALUES
		('sander16', 90, 'san francisco'),
		('sander17', 97, 'san francisco'),
		('sander18', 93, 'san francisco'),
		('sander19', 95, 'san francisco'),
		('sander20', 91, 'san francisco');`)
		if err != nil {
			t.Fatalf("Inserting rows failed: %v", err)
		}
	}
}
