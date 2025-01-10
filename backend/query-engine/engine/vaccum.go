package engine

import (
	"a2gdb/logger"
	"a2gdb/storage-engine/storage"
	"context"
	"fmt"

	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
)

const (
	ALMOST_FULL_PAGE = 486
	EIGHT_LEVEL      = 886
	SEVENTH_LEVEL    = 1286
	SIXTH_LEVEL      = 1686
	FIFITH_LEVEL     = 2086
	FOURTH_LEVEL     = 2486
	THIRD_LEVEL      = 2886
	SECOND_LEVEL     = 3286
	FIRST_LEVEL      = 3686
	EMPTY_PAGE       = 4082

	NEXT_LEVEL = 400
)

func cleanOrgnize(ctx context.Context, updateInfoChan chan ModifiedInfo, insertChan chan *NonAddedRows, tableObj *storage.TableObj, bpm *storage.BufferPoolManager) error {
	defer close(insertChan)

	dirPage := tableObj.DirectoryPage.Value

	logger.Log.Info("cleanOrgnize (start)")
	logger.Log.WithField("tableObj", tableObj.Memory).Info("Before memory separation")

	for updateInfo := range updateInfoChan {
		space := updateInfo.FreeSpaceMapping
		rowsId := updateInfo.RowIds

		bpm.Mu.Lock()
		newPage, err := storage.RearrangePAGE(space.TempPagePtr, tableObj, tableObj.TableName)
		if err != nil {
			return fmt.Errorf("RearrangePAGE failed: %w", err)
		}

		err = storage.UpdatePageInfo(rowsId, newPage, tableObj)
		if err != nil {
			return fmt.Errorf("updatePageInfo failed: %w", err)
		}

		totalSpace := newPage.Header.UpperPtr - newPage.Header.LowerPtr // just rearranged the page, so this makes sense

		logger.Log.WithFields(logrus.Fields{"totalSpace": totalSpace, "LowerPtr": newPage.Header.LowerPtr, "UpperPtr": newPage.Header.UpperPtr}).Info("Cleaned Page")

		space.TempPagePtr = nil
		memTag := getTag(space.FreeMemory)
		pageInfo := dirPage[space.PageID]

		// performance considerations
		if pageInfo.Level != 0 {
			rankSlice := tableObj.Memory[pageInfo.Level]
			tableObj.Memory[pageInfo.Level] = lo.Filter(rankSlice, func(item *storage.FreeSpace, i int) bool {
				return space.PageID != item.PageID
			})
		}

		if memTag == 0 {
			memTag = EMPTY_PAGE
		}

		pageInfo.Level = memTag
		pageInfo.ExactFreeMem = space.FreeMemory

		tableObj.Memory[memTag] = append(tableObj.Memory[memTag], space)

		err = bpm.ReplacePage(newPage) // new page was rearranged, need to update buffer pool
		if err != nil {
			return fmt.Errorf("ReplacePage failed: %w", err)
		}

		err = bpm.Unpin(storage.PageID(newPage.Header.ID), false)
		if err != nil {
			return fmt.Errorf("unpin failed: %w", err)
		}

		logger.Log.WithField("tableObj", tableObj.Memory).Info("After memory separation")
		err = saveMemMapping(tableObj)
		if err != nil {
			return fmt.Errorf("saveMemMapping failed: %w", err)
		}
		bpm.Mu.Unlock()
		insertChan <- updateInfo.NonAddedRow

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			continue
		}
	}

	logger.Log.Info("cleanOrgnize (end)")

	return nil
}

// ###[x] last two fields of pageObj aren't being saved on the updatePageInfo function but it gets add and saved here.
func memSeparationSingle(newSpace *storage.FreeSpace, tableObj *storage.TableObj) error {
	memTag := getTag(newSpace.FreeMemory)
	dirPage := tableObj.DirectoryPage.Value
	pageInfo := dirPage[newSpace.PageID]

	if memTag == 0 {
		memTag = EMPTY_PAGE
	}

	tableObj.Memory[memTag] = append(tableObj.Memory[memTag], newSpace)

	pageInfo.Level = memTag
	pageInfo.ExactFreeMem = newSpace.FreeMemory
	err := saveMemMapping(tableObj)
	if err != nil {
		return fmt.Errorf("saveMemMapping failed: %w", err)
	}

	logger.Log.WithFields(logrus.Fields{"MemTag": memTag, "ExactFreeMem": pageInfo.ExactFreeMem, "memLevel": pageInfo.Level, "offset": pageInfo.Offset}).Info("memory separation single done")

	err = storage.UpdateDirectoryPageDisk(tableObj.DirectoryPage, tableObj.DirFile)
	if err != nil {
		return fmt.Errorf("UpdateDirectoryPageDisk failed: %w", err)
	}

	logger.Log.Info("Insertion Completed")
	return nil
}

func saveMemMapping(tableObj *storage.TableObj) error {
	bytes, err := storage.EncodeMemObj(tableObj.Memory)
	if err != nil {
		return fmt.Errorf("EncodeMemObj failed: %w", err)
	}

	err = storage.WriteNonPageFile(tableObj.MemFile, bytes)
	if err != nil {
		return fmt.Errorf("WriteNonPageFile failed: %w", err)
	}

	return nil
}

func searchPage(tableObj *storage.TableObj, memoryNedded, level uint16) ([]*storage.FreeSpace, *storage.FreeSpace, uint16, int) {
	var memSlice []*storage.FreeSpace
	var memTag uint16
	var spaceInfo *storage.FreeSpace
	var deleteIndex int

	for len(memSlice) == 0 {
		memTag = getTag(level)

		//# create page
		if memTag == 0 {
			return nil, nil, memTag, 0
		}

		memSlice = tableObj.Memory[memTag]
		//# empty level
		if len(memSlice) == 0 {
			level += NEXT_LEVEL
			continue
		}

		for i, mem := range memSlice {
			if memoryNedded < mem.FreeMemory {
				spaceInfo = mem
				deleteIndex = i
				break
			}
		}

		//# free space not found
		if spaceInfo == nil {
			memSlice = []*storage.FreeSpace{}
			level += NEXT_LEVEL
			continue
		}

	}

	return memSlice, spaceInfo, memTag, deleteIndex
}

func getAvailablePage(bufferM *storage.BufferPoolManager, tableObj *storage.TableObj, memoryNedded uint16, tableName string) (*storage.PageV2, error) {
	var memSlice []*storage.FreeSpace
	var memTag uint16
	var spaceInfo *storage.FreeSpace
	var deleteIndex int

	memSlice, spaceInfo, memTag, deleteIndex = searchPage(tableObj, memoryNedded, memoryNedded)
	if memTag == 0 && spaceInfo == nil {
		logger.Log.Info("Created new page")

		page := storage.CreatePageV2(tableName)

		err := bufferM.InsertPage(page)
		if err != nil {
			return nil, fmt.Errorf("InsertPage failed: %w", err)
		}

		err = bufferM.Pin(storage.PageID(page.Header.ID))
		if err != nil {
			return nil, fmt.Errorf("pin failed: %w", err)
		}

		return page, nil
	}

	// deleting page found
	memSlice = lo.Filter(memSlice, func(item *storage.FreeSpace, i int) bool {
		return i != deleteIndex
	})
	tableObj.Memory[memTag] = memSlice

	page, err := bufferM.FetchPage(spaceInfo.PageID, tableObj)
	if err != nil {
		return nil, fmt.Errorf("FetchPage failed: %w", err)
	}

	return page, nil
}

func getTag(pageMem uint16) uint16 {
	switch {
	case pageMem <= ALMOST_FULL_PAGE:
		return ALMOST_FULL_PAGE
	case pageMem > SECOND_LEVEL && pageMem <= FIRST_LEVEL:
		return FIRST_LEVEL
	case pageMem > THIRD_LEVEL && pageMem <= SECOND_LEVEL:
		return SECOND_LEVEL
	case pageMem > FOURTH_LEVEL && pageMem <= THIRD_LEVEL:
		return THIRD_LEVEL
	case pageMem > FIFITH_LEVEL && pageMem <= FOURTH_LEVEL:
		return FOURTH_LEVEL
	case pageMem > SIXTH_LEVEL && pageMem <= FIFITH_LEVEL:
		return FIFITH_LEVEL
	case pageMem > SEVENTH_LEVEL && pageMem <= SIXTH_LEVEL:
		return SIXTH_LEVEL
	case pageMem > EIGHT_LEVEL && pageMem <= SEVENTH_LEVEL:
		return SEVENTH_LEVEL
	case pageMem > ALMOST_FULL_PAGE && pageMem <= EIGHT_LEVEL:
		return EIGHT_LEVEL
	case pageMem > FIRST_LEVEL && pageMem <= EMPTY_PAGE:
		return EMPTY_PAGE
	default:
		return 0
	}
}
