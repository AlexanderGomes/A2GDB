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

func findAndUpdate(bufferM *storage.BufferPoolManager, tableObj *storage.TableObj, bytesNeeded uint16, tableName string, encodedRows [][]byte, rowsID []uint64) error {
	page := getAvailablePage(bufferM, tableObj, bytesNeeded, tableName) // new page could've been created
	page.TABLE = storage.TableName(tableObj.TableName)

	newSpace := storage.FreeSpace{
		PageID:     storage.PageID(page.Header.ID),
		FreeMemory: page.Header.UpperPtr - page.Header.LowerPtr, //assuming new page
	}

	pageInfoObj, ok := tableObj.DirectoryPage.Value[storage.PageID(page.Header.ID)]
	if ok {
		newSpace.FreeMemory = pageInfoObj.ExactFreeMem
	}

	for _, encodedRow := range encodedRows {
		newSpace.FreeMemory -= uint16(len(encodedRow))
		err := page.AddTuple(encodedRow)
		if err != nil {
			return fmt.Errorf("failed adding row %s, for table: %s, rrror: %s", encodedRow, tableName, err)
		}
	}

	logger.Log.Info("saving page to disk (created / existing)")
	err := storage.UpdatePageInfo(rowsID, page, tableObj) // make sure to save possible new page (this is updating even already existing pages)
	if err != nil {
		return fmt.Errorf("tnternal update failed: %v", page)
	}

	logger.Log.WithFields(logrus.Fields{"newSpace": newSpace}).Info("memSeparationSingle input")
	memSeparationSingle(&newSpace, tableObj) // safe to do memory separation

	// not dirty, updated disk image before releasing
	err = bufferM.Unpin(storage.PageID(page.Header.ID), false)
	if err != nil {
		log.Fatal(err)
	}

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

func processPagesForDeletion(pages []*storage.PageV2, deleteKey, deleteVal string, tableObj *storage.TableObj) ([]*storage.FreeSpace, []uint64) {
	var freeSpaceMapping []*storage.FreeSpace
	var rowsID []uint64
	rowsID = append(rowsID, 0)

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
				rowsID = append(rowsID, row.ID)
			}
		}

		if freeSpacePage != nil {
			pageObj.ExactFreeMem = freeSpacePage.FreeMemory
			freeSpaceMapping = append(freeSpaceMapping, freeSpacePage)
		}
		logger.Log.WithFields(logrus.Fields{"Memlevel": pageObj.Level, "exactFreeMem": pageObj.ExactFreeMem, "offset": pageObj.Offset}).Info("After Modification (PageObj)")
	}

	return freeSpaceMapping, rowsID
}

type NonAddedRow struct {
	Id    uint64
	Bytes []byte
}

func processPagesForUpdate(pages []*storage.PageV2, updateKey, updateVal, filterKey, filterVal string, tableObj *storage.TableObj) ([]*storage.FreeSpace, []*NonAddedRow) {
	var freeSpaceMapping []*storage.FreeSpace
	var nonAddedRows []*NonAddedRow

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
				freeSpacePage.FreeMemory += location.Length

				nonAddedRow := NonAddedRow{
					Bytes: rowBytes,
					Id:    row.ID,
				}

				nonAddedRows = append(nonAddedRows, &nonAddedRow)
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

func handleLikeInsert(rows []*NonAddedRow, tableObj *storage.TableObj, tableName string, bpm *storage.BufferPoolManager) {
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

		var encodedBytes [][]byte
		var rowIds []uint64
		for _, row := range batch {
			bytesNeeded += len(row.Bytes)
			encodedBytes = append(encodedBytes, row.Bytes)
			rowIds = append(rowIds, row.Id)
		}

		findAndUpdate(bpm, tableObj, uint16(bytesNeeded), tableName, encodedBytes, rowIds)
	}

	logger.Log.Info("handleLikeInsert(update) Completed")
}
