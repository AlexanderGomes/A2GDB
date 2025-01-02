package engine

import (
	"a2gdb/logger"
	"a2gdb/storage-engine/storage"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

func (qe *QueryEngine) GetTableObj(tableName string) (*storage.TableObj, error) {
	var tableObj *storage.TableObj
	var err error
	manager := qe.StorageManager

	tableObj, found := manager.TableObjs[storage.TableName(tableName)]
	if !found {
		tableObj, err = manager.InMemoryTableSetUp(tableName)
		if err != nil {
			return nil, fmt.Errorf("GetTable: %w", err)
		}
	}

	return tableObj, err
}

func prepareRows(plan map[string]interface{}, selectedCols []interface{}, tableName, primary string) (uint16, []uint64, [][]byte) {
	var bytesNeeded uint16
	rowsID := []uint64{}
	encodedRows := [][]byte{}

	interfaceRows := plan["rows"].([]interface{})

	for _, row := range interfaceRows {
		newRow := storage.RowV2{
			ID:     storage.GenerateRandomID(),
			Values: make(map[string]string),
		}

		//#Add row values
		newRow.Values[primary] = strconv.FormatUint(newRow.ID, 10)
		for i, rowVal := range row.([]interface{}) {
			strRowVal := strings.ReplaceAll(rowVal.(string), "'", "")
			strRowCol := selectedCols[i].(string)

			newRow.Values[strRowCol] = strRowVal
		}

		encodedRow, err := storage.EncodeRow(&newRow)
		if err != nil {
			log.Printf("Failed Encoding row %v For Table: %s", row, tableName)
		}

		bytesNeeded += uint16(len(encodedRow))
		encodedRows = append(encodedRows, encodedRow)
		rowsID = append(rowsID, newRow.ID)
	}

	return bytesNeeded, rowsID, encodedRows
}

func findAndUpdate(tableObj *storage.TableObj, bytesNeeded uint16, tableName string, encodedRows [][]byte, rowsID []uint64) error {
	page := getAvailablePage(tableObj, bytesNeeded) // new page could've been created

	for _, encodedRow := range encodedRows {
		err := page.AddTuple(encodedRow)
		if err != nil {
			return fmt.Errorf("failed adding row %s, for table: %s, rrror: %s", encodedRow, tableName, err)
		}
	}

	logger.Log.Info("saving page to disk (created / existing)")
	err := updatePageInfo(rowsID, page, tableObj) // make sure to save possible new page (this is updating even already existing pages)
	if err != nil {
		return fmt.Errorf("tnternal update failed: %v", page)
	}

	availableSpace := page.Header.UpperPtr - page.Header.LowerPtr
	newSpace := storage.FreeSpace{PageID: storage.PageID(page.Header.ID), FreeMemory: availableSpace}

	logger.Log.WithFields(logrus.Fields{"newSpace": newSpace}).Info("memSeparationSingle input")
	memSeparationSingle(&newSpace, tableObj) // safe to do memory separation
	return nil
}

func checkPresenceGetPrimary(selectedCols []interface{}, tableName string, catalog *storage.Catalog) (string, error) {
	var primary string

	// #check if table exist
	tableInfo, ok := catalog.Tables[storage.TableName(tableName)]
	if !ok {
		return "", fmt.Errorf("table: %s doesn't exist", tableName)
	}

	// #check if cols exist
	for _, selectedCol := range selectedCols {
		selectedCol := selectedCol.(string)

		_, ok := tableInfo.Schema[selectedCol]
		if !ok {
			return "", fmt.Errorf("column: %s on table: %s doesn't exist", selectedCol, tableName)
		}
	}

	//#get primary
	for column, columnInfo := range tableInfo.Schema {
		if columnInfo.IsIndex {
			primary = column
		}
	}

	if primary == "" {
		return "", fmt.Errorf("primary doesn't exist")
	}

	return primary, nil
}

func updatePageInfo(rowsID []uint64, pageFound *storage.PageV2, tableObj *storage.TableObj) error {
	pageID := storage.PageID(pageFound.Header.ID)
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

	if err := storage.UpdateBp(rowsID, *tableObj, *pageInfObj); err != nil {
		return fmt.Errorf("update B+ tree error: %w", err)
	}

	return nil
}

func (qe *QueryEngine) getAllRows(tableName string) []*storage.RowV2 {
	var rows []*storage.RowV2

	tableObj, err := qe.GetTableObj(tableName)
	if err != nil {
		log.Fatalf("GetTable failed for: %s, error: %s", tableName, err)
	}

	directoryMap := tableObj.DirectoryPage.Value
	pages, err := storage.GetTablePages(tableObj.DataFile, nil)
	if err != nil {
		log.Fatalf("GetTablePages failed for: %s, error: %s", tableName, err)
	}

	for _, page := range pages {
		pageId := storage.PageID(page.Header.ID)
		pageObj, ok := directoryMap[pageId]

		if !ok {
			log.Fatalf("PageObj not found for page: %v", page.Header.ID)
		}

		for _, location := range pageObj.PointerArray {
			if location.Free {
				continue
			}

			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				log.Fatalf("DecodeRow error: %s", err)
			}

			rows = append(rows, row)
		}
	}

	return rows
}

func processPagesForDeletion(pages []*storage.PageV2, deleteKey, deleteVal string, tableObj *storage.TableObj) []*storage.FreeSpace {
	var freeSpaceMapping []*storage.FreeSpace

	for _, page := range pages {
		var freeSpacePage *storage.FreeSpace
		pageObj := tableObj.DirectoryPage.Value[storage.PageID(page.Header.ID)]

		logger.Log.WithFields(logrus.Fields{"Memlevel": pageObj.Level, "exactFreeMem": pageObj.ExactFreeMem, "offset": pageObj.Offset}).Info("Before Modification (PageObj)")
		for i := range pageObj.PointerArray {
			location := &pageObj.PointerArray[i]
			if location.Free {
				continue
			}

			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				log.Panicf("couldn't decode row, location: %+v, error: %s", location, err)
			}

			if row.Values[deleteKey] == deleteVal {
				if freeSpacePage == nil {
					freeSpacePage = &storage.FreeSpace{
						PageID:      storage.PageID(page.Header.ID),
						TempPagePtr: page,
						FreeMemory:  pageObj.ExactFreeMem}
				}

				freeSpacePage.FreeMemory += location.Length
				location.Free = true
			}
		}

		if freeSpacePage != nil {
			pageObj.ExactFreeMem = freeSpacePage.FreeMemory
			freeSpaceMapping = append(freeSpaceMapping, freeSpacePage)
		}
		logger.Log.WithFields(logrus.Fields{"Memlevel": pageObj.Level, "exactFreeMem": pageObj.ExactFreeMem, "offset": pageObj.Offset}).Info("After Modification (PageObj)")
	}

	return freeSpaceMapping
}

func processPagesForUpdate(pages []*storage.PageV2, updateKey, updateVal, filterKey, filterVal string, tableObj *storage.TableObj) ([]*storage.FreeSpace, [][]byte) {
	var freeSpaceMapping []*storage.FreeSpace
	var nonAddedRows [][]byte

	for _, page := range pages {
		var freeSpacePage *storage.FreeSpace

		pageId := storage.PageID(page.Header.ID)
		pageObj := tableObj.DirectoryPage.Value[pageId]

		logger.Log.WithFields(logrus.Fields{"Memlevel": pageObj.Level, "exactFreeMem": pageObj.ExactFreeMem, "offset": pageObj.Offset}).Info("Before Modification (PageObj)")
		for i := range pageObj.PointerArray {
			location := &pageObj.PointerArray[i]

			if location.Free {
				continue
			}

			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				log.Panicf("couldn't decode row, location: %+v, error: %s", location, err)
			}

			if row.Values[filterKey] == filterVal {
				if freeSpacePage == nil {
					freeSpacePage = &storage.FreeSpace{PageID: storage.PageID(page.Header.ID), TempPagePtr: page, FreeMemory: pageObj.ExactFreeMem}
				}

				row.Values[updateKey] = updateVal
				rowBytes, err := storage.EncodeRow(row)
				if err != nil {
					log.Panicf("couldn't encode row %+v, error: %s", row, err)
				}

				location.Free = true
				freeSpacePage.FreeMemory -= location.Length

				nonAddedRows = append(nonAddedRows, rowBytes)
			}
		}

		if freeSpacePage != nil {
			freeSpaceMapping = append(freeSpaceMapping, freeSpacePage)
			pageObj.ExactFreeMem = freeSpacePage.FreeMemory
		}

		logger.Log.WithFields(logrus.Fields{"Memlevel": pageObj.Level, "exactFreeMem": pageObj.ExactFreeMem, "offset": pageObj.Offset}).Info("After Modification (PageObj)")
	}

	return freeSpaceMapping, nonAddedRows
}

func handleLikeInsert(rows [][]byte, tableObj *storage.TableObj, tableName string) {
	logger.Log.Info("handleLikeInsert(update) Started")

	batchSize := 5
	totalRows := len(rows)

	for i := 0; i < totalRows; i += batchSize {
		end := i + batchSize
		if end > totalRows {
			end = totalRows
		}

		logger.Log.Infof("processing row batches from %d to %d", i, end-1)
		batch := rows[i:end]
		bytesNeeded := 0
		for _, row := range batch {
			bytesNeeded += len(row)
		}

		findAndUpdate(tableObj, uint16(bytesNeeded), tableName, batch, nil)
	}

	logger.Log.Info("handleLikeInsert(update) Completed")
}
