package engine

import (
	"a2gdb/logger"
	"a2gdb/storage-engine/storage"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

func prepareRows(plan map[string]interface{}, selectedCols []interface{}, primary string) (uint16, [][]byte, error) {
	var bytesNeeded uint16
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
			return 0, nil, fmt.Errorf("encodeRow failed: %w", err)
		}

		bytesNeeded += uint16(len(encodedRow))
		encodedRows = append(encodedRows, encodedRow)
	}

	return bytesNeeded, encodedRows, nil
}

func findAndUpdate(bufferM *storage.BufferPoolManager, tableObj *storage.TableObj, tableStats *storage.TableInfo, bytesNeeded uint16, tableName string, encodedRows [][]byte) error {
	page, err := getAvailablePage(bufferM, tableObj, bytesNeeded, tableName) // new page could've been created
	if err != nil {
		return fmt.Errorf("getAvailablePage failed: %w", err)
	}

	newSpace := storage.FreeSpace{
		PageID:     storage.PageID(page.Header.ID),
		FreeMemory: page.Header.UpperPtr - page.Header.LowerPtr, //assuming new page
	}

	tableObj.DirectoryPage.Mu.RLock()
	pageInfoObj, ok := tableObj.DirectoryPage.Value[storage.PageID(page.Header.ID)]
	tableObj.DirectoryPage.Mu.RUnlock()
	if ok {
		pageInfoObj.Mu.RLock()
		newSpace.FreeMemory = pageInfoObj.ExactFreeMem
		pageInfoObj.Mu.RUnlock()
	}

	for _, encodedRow := range encodedRows {
		newSpace.FreeMemory -= uint16(len(encodedRow))
		err := page.AddTuple(encodedRow)
		if err != nil {
			return fmt.Errorf("AddTuple failed: %w", err)
		}
	}

	logger.Log.Info("saving page to disk (created / existing)")
	err = storage.UpdatePageInfo(page, tableObj, tableStats, bufferM.DiskManager) // make sure to save possible new page (this is updating even already existing pages)
	if err != nil {
		return fmt.Errorf("UpdatePageInfo failed: %v", page)
	}

	logger.Log.WithFields(logrus.Fields{"newSpace": newSpace}).Info("memSeparationSingle input")
	err = memSeparationSingle(&newSpace, tableObj) // safe to do memory separation
	if err != nil {
		return fmt.Errorf("memSeparationSingle failed: %v", page)
	}

	return nil
}

func checkPresenceGetPrimary(selectedCols []interface{}, tableName string, catalog *storage.Catalog) (string, error) {
	var primary string

	// #check if table exist
	tableInfo, ok := catalog.Tables[tableName]
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

func processPagesForDeletion(ctx context.Context, pages chan *storage.PageV2, updateInfoChan chan ModifiedInfo, deleteKey, deleteVal string, tableObj *storage.TableObj) error {
	logger.Log.Info("processPagesForDeletion (start)")
	defer close(updateInfoChan)

	for page := range pages {
		var freeSpacePage *storage.FreeSpace
		var updateInfo ModifiedInfo

		tableObj.DirectoryPage.Mu.RLock()
		pageObj := tableObj.DirectoryPage.Value[storage.PageID(page.Header.ID)]
		pageObj.Mu.Lock()
		tableObj.DirectoryPage.Mu.RUnlock()

		logger.Log.WithFields(logrus.Fields{"Memlevel": pageObj.Level, "exactFreeMem": pageObj.ExactFreeMem, "offset": pageObj.Offset}).Info("Before Modification (PageObj)")
		for i := range pageObj.PointerArray {
			location := &pageObj.PointerArray[i]
			if location.Free {
				continue
			}

			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				return fmt.Errorf("DecodeRow failed: %w", err)
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
			pageObj.Mu.Unlock()
			updateInfo.FreeSpaceMapping = freeSpacePage
			updateInfoChan <- updateInfo
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			continue
		}
	}

	logger.Log.Info("processPagesForDeletion (end)")
	return nil
}

type NonAddedRows struct {
	BytesNeeded uint16
	Rows        [][]byte
}

type ModifiedInfo struct {
	FreeSpaceMapping *storage.FreeSpace
	NonAddedRow      *NonAddedRows
}

func processPagesForUpdate(ctx context.Context, pageChan chan *storage.PageV2, updateInfoChan chan ModifiedInfo, updateKey, updateVal, filterKey, filterVal string, tableObj *storage.TableObj) error {
	logger.Log.Info("processPagesForUpdate (start)")
	defer close(updateInfoChan)

	for page := range pageChan {
		var freeSpacePage *storage.FreeSpace
		var updateInfo ModifiedInfo
		var nonAddedRows NonAddedRows

		pageId := storage.PageID(page.Header.ID)

		directoryPage := tableObj.DirectoryPage

		directoryPage.Mu.RLock()
		pageObj := directoryPage.Value[pageId]
		pageObj.Mu.Lock()

		directoryPage.Mu.RUnlock()

		logger.Log.WithFields(logrus.Fields{"Memlevel": pageObj.Level, "exactFreeMem": pageObj.ExactFreeMem, "offset": pageObj.Offset}).Info("Before Modification (PageObj)")
		for i := range pageObj.PointerArray {
			location := &pageObj.PointerArray[i]

			if location.Free {
				continue
			}

			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := storage.DecodeRow(rowBytes)
			if err != nil {
				return fmt.Errorf("couldn't decode row, location: %+v, error: %s", location, err)
			}

			if row.Values[filterKey] == filterVal {
				if freeSpacePage == nil {
					freeSpacePage = &storage.FreeSpace{PageID: storage.PageID(page.Header.ID), TempPagePtr: page, FreeMemory: pageObj.ExactFreeMem}
				}

				row.Values[updateKey] = updateVal
				rowBytes, err := storage.EncodeRow(row)
				if err != nil {
					return fmt.Errorf("EncodeRow failed: %w", err)
				}

				location.Free = true
				freeSpacePage.FreeMemory += location.Length
				nonAddedRows.BytesNeeded += uint16(len(rowBytes))

				nonAddedRows.Rows = append(nonAddedRows.Rows, rowBytes)
			}
		}

		if freeSpacePage != nil {
			pageObj.Mu.Unlock()
			updateInfo.FreeSpaceMapping = freeSpacePage
			updateInfo.NonAddedRow = &nonAddedRows

			logger.Log.WithField("updateInfo", updateInfo).Info("Page processed")
			updateInfoChan <- updateInfo
		}

		logger.Log.WithFields(logrus.Fields{"Memlevel": pageObj.Level, "exactFreeMem": pageObj.ExactFreeMem, "offset": pageObj.Offset}).Info("After Modification (PageObj)")

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			continue
		}
	}

	logger.Log.Info("processPagesForUpdate (end)")
	return nil
}

func handleLikeInsert(ctx context.Context, nonAddedRows chan *NonAddedRows, tableObj *storage.TableObj, tableName string, bpm *storage.BufferPoolManager, tableStats *storage.TableInfo) error {
	logger.Log.Info("handleLikeInsert(update) Started")

	for nonAddedRow := range nonAddedRows {
		if nonAddedRow.BytesNeeded >= AVAIL_DATA {
			chunkedRows := ChunkRows(nonAddedRow)

			for _, chunkedRow := range chunkedRows {
				err := findAndUpdate(bpm, tableObj, tableStats, chunkedRow.BytesNeeded, tableName, chunkedRow.Rows)
				if err != nil {
					return fmt.Errorf("findAndUpdate failed: %w", err)
				}
			}
			continue
		}

		err := findAndUpdate(bpm, tableObj, tableStats, nonAddedRow.BytesNeeded, tableName, nonAddedRow.Rows)
		if err != nil {
			return fmt.Errorf("findAndUpdate failed: %w", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			continue
		}
	}

	logger.Log.Info("handleLikeInsert(update) Completed")
	return nil
}

func ChunkRows(nonAddedRows *NonAddedRows) []*NonAddedRows {
	const maxBytesPerChunk = 2096
	var chunkedRows []*NonAddedRows

	currentChunk := &NonAddedRows{}
	for _, row := range nonAddedRows.Rows {
		rowSize := uint16(len(row))

		if currentChunk.BytesNeeded+rowSize >= maxBytesPerChunk {
			chunkedRows = append(chunkedRows, currentChunk)
			currentChunk = &NonAddedRows{}
		}

		currentChunk.BytesNeeded += rowSize
		currentChunk.Rows = append(currentChunk.Rows, row)
	}

	if len(currentChunk.Rows) > 0 {
		chunkedRows = append(chunkedRows, currentChunk)
	}

	return chunkedRows
}
