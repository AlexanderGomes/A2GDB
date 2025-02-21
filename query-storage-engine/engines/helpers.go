package engines

import (
	"a2gdb/logger"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

func prepareRows(plan map[string]interface{}, selectedCols []interface{}, primary, tableName, txID string, wal *WalManager, transactionOff bool) (uint16, [][]byte, error) {
	var bytesNeeded uint16
	var encodedRows [][]byte

	interfaceRows := plan["rows"].([]interface{})

	for _, row := range interfaceRows {
		newRow := RowV2{
			ID:     GenerateRandomID(),
			Values: make(map[string]string),
		}

		//#Add row values
		newRow.Values[primary] = strconv.FormatUint(newRow.ID, 10)
		for i, rowVal := range row.([]interface{}) {
			strRowVal := strings.ReplaceAll(rowVal.(string), "'", "")
			strRowCol := selectedCols[i].(string)

			newRow.Values[strRowCol] = strRowVal
		}

		encodedRow, err := EncodeRow(&newRow)
		if err != nil {
			return 0, nil, fmt.Errorf("encodeRow failed: %w", err)
		}

		if !transactionOff {
			err = wal.Log(txID, LogTypeInsert, tableName, newRow.ID, nil, encodedRow)
			if err != nil {
				return 0, nil, fmt.Errorf("wal.log failed: %w", err)
			}
		}

		bytesNeeded += uint16(len(encodedRow))
		encodedRows = append(encodedRows, encodedRow)
	}

	return bytesNeeded, encodedRows, nil
}

func findAndUpdate(bufferM *BufferPoolManager, tableObj *TableObj, tableStats *TableInfo, bytesNeeded uint16, tableName string, encodedRows [][]byte) error {
	page, err := getAvailablePage(bufferM, tableObj, bytesNeeded, tableName) // new page could've been created
	if err != nil {
		return fmt.Errorf("getAvailablePage failed: %w", err)
	}

	newSpace := FreeSpace{
		PageID:     PageID(page.Header.ID),
		FreeMemory: page.Header.UpperPtr - page.Header.LowerPtr, //assuming new page
	}

	tableObj.DirectoryPage.Mu.RLock()
	pageInfoObj, ok := tableObj.DirectoryPage.Value[PageID(page.Header.ID)]
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
	err = UpdatePageInfo(page, tableObj, tableStats, bufferM.DiskManager, ADDING) // make sure to save possible new page (this is updating even already existing pages)
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

func isPrimary(key string, tableName string, catalog *Catalog) (bool, error) {
	tableInfo, ok := catalog.Tables[tableName]
	if !ok {
		return false, fmt.Errorf("table: %s doesn't exist", tableName)
	}

	columnInfo := tableInfo.Schema[key]

	return columnInfo.IsIndex, nil
}

func checkPresenceGetPrimary(selectedCols []interface{}, tableName string, catalog *Catalog) (string, error) {
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

func processPagesForDeletion(ctx context.Context, pages chan *PageV2, updateInfoChan chan ModifiedInfo, deleteKey, deleteVal, txID string, isPrimary bool, tableObj *TableObj, wal *WalManager, txOff bool) error {
	defer close(updateInfoChan)

	var foundMatch bool
	for page := range pages {
		if isPrimary && foundMatch {
			break
		}

		var freeSpacePage *FreeSpace
		var updateInfo ModifiedInfo

		tableObj.DirectoryPage.Mu.RLock()
		pageObj, ok := tableObj.DirectoryPage.Value[PageID(page.Header.ID)]
		if !ok {
			return errors.New("pageObj missing")
		}
		tableObj.DirectoryPage.Mu.RUnlock()

		pageObj.Mu.Lock()

		for i := range pageObj.PointerArray {
			location := &pageObj.PointerArray[i]
			if location.Free {
				continue
			}

			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := DecodeRow(rowBytes)
			if err != nil {
				return fmt.Errorf("DecodeRow failed: %w", err)
			}

			if row.Values[deleteKey] == deleteVal {
				if freeSpacePage == nil {
					freeSpacePage = &FreeSpace{
						PageID:      PageID(page.Header.ID),
						TempPagePtr: page,
						FreeMemory:  pageObj.ExactFreeMem}
				}

				if !txOff {
					err = wal.Log(txID, LogTypeDelete, tableObj.TableName, row.ID, rowBytes, nil)
					if err != nil {
						return fmt.Errorf("wal.log failed: %w", err)
					}
				}

				freeSpacePage.FreeMemory += location.Length
				location.Free = true

				if isPrimary {
					foundMatch = true
					break
				}
			}
		}

		pageObj.Mu.Unlock()
		if freeSpacePage != nil {
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

	return nil
}

type NonAddedRows struct {
	BytesNeeded uint16
	Rows        [][]byte
}

type ModifiedInfo struct {
	FreeSpaceMapping *FreeSpace
	NonAddedRow      *NonAddedRows
}

func processPagesForUpdate(ctx context.Context, pageChan chan *PageV2, updateInfoChan chan ModifiedInfo, updateKey, updateVal, filterKey, filterVal, txID string, tableObj *TableObj, wal *WalManager, txOff bool) error {
	logger.Log.Info("processPagesForUpdate (start)")
	defer close(updateInfoChan)

	for page := range pageChan {
		var freeSpacePage *FreeSpace
		var updateInfo ModifiedInfo
		var nonAddedRows NonAddedRows

		pageId := PageID(page.Header.ID)

		directoryPage := tableObj.DirectoryPage

		directoryPage.Mu.RLock()
		pageObj := directoryPage.Value[pageId]
		directoryPage.Mu.RUnlock()
		pageObj.Mu.Lock()

		logger.Log.WithFields(logrus.Fields{"Memlevel": pageObj.Level, "exactFreeMem": pageObj.ExactFreeMem, "offset": pageObj.Offset}).Info("Before Modification (PageObj)")
		for i := range pageObj.PointerArray {
			location := &pageObj.PointerArray[i]

			if location.Free {
				continue
			}

			oldRowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := DecodeRow(oldRowBytes)
			if err != nil {
				return fmt.Errorf("couldn't decode row, location: %+v, error: %s", location, err)
			}

			if row.Values[filterKey] == filterVal {
				if freeSpacePage == nil {
					freeSpacePage = &FreeSpace{PageID: PageID(page.Header.ID), TempPagePtr: page, FreeMemory: pageObj.ExactFreeMem}
				}

				row.Values[updateKey] = updateVal
				newRowBytes, err := EncodeRow(row)
				if err != nil {
					return fmt.Errorf("EncodeRow failed: %w", err)
				}

				if !txOff {
					err = wal.Log(txID, LogTypeUpdate, tableObj.TableName, row.ID, oldRowBytes, newRowBytes)
					if err != nil {
						return fmt.Errorf("wal.log failed: %w", err)
					}
				}

				location.Free = true
				freeSpacePage.FreeMemory += location.Length
				nonAddedRows.BytesNeeded += uint16(len(newRowBytes))

				nonAddedRows.Rows = append(nonAddedRows.Rows, newRowBytes)
			}
		}

		pageObj.Mu.Unlock() // at the end of each page

		if freeSpacePage != nil {
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

func handleLikeInsert(ctx context.Context, nonAddedRows chan *NonAddedRows, tableObj *TableObj, tableName string, bpm *BufferPoolManager, tableStats *TableInfo) error {
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

func getPrimary(tableName string, catalog *Catalog) (string, error) {
	var primary string

	tableInfo, ok := catalog.Tables[tableName]
	if !ok {
		return "", fmt.Errorf("table: %s doesn't exist", tableName)
	}

	for column, columnInfo := range tableInfo.Schema {
		if columnInfo.IsIndex {
			primary = column
			break
		}
	}

	if primary == "" {
		return "", fmt.Errorf("primary doesn't exist")
	}

	return primary, nil
}

// &name=User&schema=[&Email=VARCHAR&Password=VARCHAR&UserId=PRIMARY KEY&Name=VARCHAR]&auth=[&userId=12244126150183881992&dbName=NEWDB]

// steps
// 5. why do we need "&" at the end ? [x]
// 6. develop proper initialization and ending of a body request. [x]

// 1. initiaze and close the collection of slices
// 2. collect the value inside of the slices as normal already implemented key:val
// 3. change return value to map[string][]string
// 4. change how key:val are being collected in the case that slice collection is on.

func ParsingRegistration(stringfied string) map[string]string {
	var currKey []rune
	var currVal []rune
	fields := make(map[string]string)

	var collectingKey bool
	var collectingVal bool

	for _, char := range stringfied {
		if char == '&' { // only collect previous &key=val when a & is seen again.
			if len(currKey) > 0 && len(currVal) > 0 {
				key := string(currKey)
				val := string(currVal)

				fields[key] = val
				currKey = []rune{}
				currVal = []rune{}
			}

			collectingKey = true
			collectingVal = false
			continue
		}

		if char == '=' {
			collectingKey = false
			collectingVal = true
			continue
		}

		if collectingKey {
			currKey = append(currKey, char)
		}

		if collectingVal {
			currVal = append(currVal, char)
		}
	}

	return fields
}

func handleError(err error, msg string) Result {
	return Result{
		Error: err,
		Msg:   msg,
	}
}

func rollbackAndReturn(txId, primary, modifiedColumn string, walManager *WalManager, engine *QueryEngine, catalog *Catalog, err error, msg string) Result {
	if rollbackErr := walManager.AbortTransaction(txId, primary, modifiedColumn, engine, catalog); rollbackErr != nil {
		err = fmt.Errorf("AbortTransaction failed: %w", rollbackErr)
	}
	return Result{
		Error: err,
		Msg:   msg,
	}
}
