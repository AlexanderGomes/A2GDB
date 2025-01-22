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
	AVAIL_DATA       = 4068

	NEXT_LEVEL = 400
)

func cleanOrgnize(ctx context.Context, updateInfoChan chan ModifiedInfo, insertChan chan *NonAddedRows, tableObj *storage.TableObj, tableStats *storage.TableInfo) error {
	if insertChan != nil {
		defer close(insertChan)
	}

	for updateInfo := range updateInfoChan {
		space := updateInfo.FreeSpaceMapping

		newPage, err := storage.RearrangePAGE(space.TempPagePtr, tableObj, tableObj.TableName)
		if err != nil {
			return fmt.Errorf("RearrangePAGE failed: %w", err)
		}

		space.TempPagePtr = nil
		memTag := getTag(space.FreeMemory)
		if memTag == 0 {
			memTag = EMPTY_PAGE
		}

		dirPage := tableObj.DirectoryPage

		dirPage.Mu.RLock()
		pageObj := dirPage.Value[space.PageID]
		dirPage.Mu.RUnlock()

		pageObj.Mu.Lock()
		if pageObj.Level != 0 {
			tableObj.Mu.Lock()
			rankSlice := tableObj.Memory[pageObj.Level]
			tableObj.Memory[pageObj.Level] = lo.Filter(rankSlice, func(item *storage.FreeSpace, i int) bool {
				return space.PageID != item.PageID
			})
			tableObj.Mu.Unlock()
		}

		pageObj.Level = memTag
		pageObj.ExactFreeMem = space.FreeMemory
		pageObj.Mu.Unlock()

		err = storage.UpdatePageInfo(newPage, tableObj, tableStats, nil)
		if err != nil {
			return fmt.Errorf("updatePageInfo failed: %w", err)
		}

		tableObj.Mu.Lock()
		tableObj.Memory[memTag] = append(tableObj.Memory[memTag], space)

		err = saveMemMapping(tableObj)
		if err != nil {
			return fmt.Errorf("saveMemMapping failed: %w", err)
		}

		tableObj.Mu.Unlock()

		if insertChan != nil {
			insertChan <- updateInfo.NonAddedRow
		}

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

	dirPage := tableObj.DirectoryPage

	dirPage.Mu.Lock()
	pageObj := dirPage.Value[newSpace.PageID]
	pageObj.Mu.Lock()

	if memTag == 0 {
		memTag = EMPTY_PAGE
	}

	tableObj.Mu.Lock()
	tableObj.Memory[memTag] = append(tableObj.Memory[memTag], newSpace)
	err := saveMemMapping(tableObj)
	if err != nil {
		return fmt.Errorf("saveMemMapping failed: %w", err)
	}
	tableObj.Mu.Unlock()

	pageObj.Level = memTag
	pageObj.ExactFreeMem = newSpace.FreeMemory
	logger.Log.WithFields(logrus.Fields{"MemTag": memTag, "ExactFreeMem": pageObj.ExactFreeMem, "memLevel": pageObj.Level, "offset": pageObj.Offset}).Info("memory separation single done")
	pageObj.Mu.Unlock()

	err = storage.UpdateDirectoryPageDisk(tableObj.DirectoryPage, tableObj.DirFile)
	if err != nil {
		return fmt.Errorf("UpdateDirectoryPageDisk failed: %w", err)
	}
	dirPage.Mu.Unlock()

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

		tableObj.Mu.RLock()
		memSlice = tableObj.Memory[memTag]
		tableObj.Mu.RUnlock()

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

		return page, nil
	}

	// deleting page found
	memSlice = lo.Filter(memSlice, func(item *storage.FreeSpace, i int) bool {
		return i != deleteIndex
	})

	tableObj.Mu.Lock()
	tableObj.Memory[memTag] = memSlice
	tableObj.Mu.Unlock()

	page, err := bufferM.FetchPage(spaceInfo.PageID, tableObj)
	if err != nil {
		return nil, fmt.Errorf("FetchPage failed: %w", err)
	}

	page.TABLE = tableObj.TableName

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
