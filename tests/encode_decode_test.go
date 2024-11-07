package tests

import (
	"disk-db/storage"
	"fmt"
	"testing"
)

func TestDirectoryEncodeDecode(t *testing.T) {
	oldDirectory := storage.DirectoryPageV2{Value: make(map[storage.PageID]*storage.PageInfo)}
	before_encoding_pageInfo := storage.PageInfo{
		Offset:       91929912912,
		PointerArray: []storage.TupleLocation{{Offset: 2020, Length: 249}, {Offset: 2320, Length: 429}, {Offset: 2423, Length: 129}},
	}

	pageId := storage.PageID(19929101022)
	oldDirectory.Value[pageId] = &before_encoding_pageInfo

	encodedDir, err := storage.EncodeDirectory(&oldDirectory)
	if err != nil {
		t.Fatalf("failed encoding directory: %v", err)
	}

	decoded_directory, err := storage.DecodeDirectory(encodedDir)
	if err != nil {
		t.Fatalf("failed decoding directory: %v", err)
	}

	for key, pageObj := range decoded_directory.Value {
		for oldKey, oldPageObj := range oldDirectory.Value {
			if key != oldKey {
				t.Fatal("directory: different page IDS")
			}

			if pageObj.Offset != oldPageObj.Offset {
				t.Fatal("directory: different offsets")
			}

			for i, tupleLocation := range pageObj.PointerArray {
				old_tuple_location := oldPageObj.PointerArray[i]
				if tupleLocation != old_tuple_location {
					t.Fatal("directory: tuple location inconsistency")
				}
			}

			for i, val := range pageObj.FSM {
				oldVal := oldPageObj.FSM[i]
				if val != oldVal {
					t.Fatal("directory: FSM inconsistency")
				}
			}
		}
	}
}

func TestRowEncodeDecode(t *testing.T) {
	oldRow := &storage.RowV2{Values: make(map[string]string)}

	oldRow.ID = 91929192912
	oldRow.Values["Username"] = "'sander12'"
	oldRow.Values["UserId"] = fmt.Sprint(oldRow.ID)
	oldRow.Values["Age"] = "29"
	oldRow.Values["Company"] = "Apple"

	encoded_row, err := storage.SerializeRow(oldRow)
	if err != nil {
		t.Fatalf("encoding row failed: %v", err)
	}

	decoded_row, err := storage.DecodeRow(encoded_row)
	if err != nil {
		t.Fatalf("decoding row failed: %v", err)
	}

	if decoded_row.ID != oldRow.ID {
		t.Fatalf("different IDS")
	}

	for key, val := range decoded_row.Values {
		oldVal, ok := oldRow.Values[key]
		if !ok || oldVal != val {
			t.Fatalf("field mismatch")
		}
	}
}

func TestPageEncodeDecode(t *testing.T) {
	var oldPage storage.PageV2
	oldHeader := storage.PageHeader{
		ID:        1929192912,
		LowerPtr:  storage.HeaderSize,
		UpperPtr:  storage.PageDataSize,
		NumTuples: 0,
	}

	oldPage.Header = oldHeader
	oldPage.Data = make([]byte, storage.PageDataSize)

	oldRow := &storage.RowV2{Values: make(map[string]string)}
	oldRow.ID = 91929192912
	oldRow.Values["Username"] = "'sander12'"
	oldRow.Values["UserId"] = fmt.Sprint(oldRow.ID)
	oldRow.Values["Age"] = "29"
	oldRow.Values["Company"] = "Apple"

	bytes, err := storage.SerializeRow(oldRow)
	if err != nil {
		t.Fatalf("encoding row failed: %v", err)
	}

	oldPage.AddTuple(bytes)

	encoded_page, err := storage.EncodePageV2(&oldPage)
	if err != nil {
		t.Fatalf("encoding page failed: %v", err)
	}

	decoded_page, err := storage.DecodePageV2(encoded_page)
	if err != nil {
		t.Fatalf("decoding page failed: %v", err)
	}

	if decoded_page.Header != oldPage.Header {
		t.Fatal("header inconsistency")
	}

	for _, location := range oldPage.PointerArray {
		rowBytes := decoded_page.Data[location.Offset : location.Offset+location.Length]
		row, err := storage.DecodeRow(rowBytes)
		if err != nil {
			t.Fatalf("error decoding row: %v", err)
		}

		if row.ID == 0 || row.ID != oldRow.ID {
			t.Fatal("different IDs")
		}

		for key, val := range row.Values {
			oldVal, ok := oldRow.Values[key]
			if !ok {
				t.Fatal("row mismatch, missing fields")
			}

			if oldVal != val {
				t.Fatal("row mismatch, different values")
			}
		}
	}
}

func TestCatalogEncodeDecode(t *testing.T) {
	tableInfo := storage.TableInfo{Schema: make(map[string]storage.ColumnType)}
	tableInfo.Schema["UserId"] = storage.ColumnType{Type: "uint64", IsIndex: true}
	tableInfo.Schema["Username"] = storage.ColumnType{Type: "VARCHAR", IsIndex: false}
	tableInfo.Schema["Email"] = storage.ColumnType{Type: "VARCHAR", IsIndex: false}

	old_catalog := storage.Catalog{Tables: make(map[storage.TableName]*storage.TableInfo)}
	old_catalog.Tables["User"] = &tableInfo

	encoded_catalog, err := storage.SerializeCatalog(&old_catalog)
	if err != nil {
		t.Fatalf("error encoding catalog: %v", err)
	}

	new_catalog, err := storage.DeserializeCatalog(encoded_catalog)
	if err != nil {
		t.Fatalf("error decoding catalog: %v", err)
	}

	for tableName, tableInfo := range new_catalog.Tables {
		old_table_info, ok := old_catalog.Tables[tableName]
		if !ok {
			t.Fatal("table not found")
		}

		if old_table_info.NumOfPages != tableInfo.NumOfPages {
			t.Fatal("number of pages mismatch")
		}

		for key, val := range tableInfo.Schema {
			oldVal, ok := old_table_info.Schema[key]
			if !ok {
				t.Fatal("catalog table mismatch")
			}

			if val != oldVal {
				t.Fatalf("different columsn types")
			}
		}
	}
}
