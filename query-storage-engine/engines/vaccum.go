package engines

import (
	"a2gdb/logger"
	"context"
	"fmt"
	"reflect"

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
	AVAIL_DATA       = 4068

	NEXT_LEVEL = 400
)

func cleanOrgnize(ctx context.Context, accountingCtx *MemoryContext, updateInfoChan chan *ModifiedInfo, insertChan chan *NonAddedRows, tableObj *TableObj, tableStats *TableInfo) error {
	logger.Log.Info("cleanOrgnize (start)")

	if insertChan != nil {
		defer close(insertChan)
	}

	for updateInfo := range updateInfoChan {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		space := updateInfo.FreeSpaceMapping

		newPage, err := RearrangePAGE(space.TempPagePtr, tableObj, tableObj.TableName)
		if err != nil {
			return fmt.Errorf("(cleanOrgnize) => RearrangePAGE failed: %w", err)
		}

		space.TempPagePtr = nil
		memTag := getTag(space.FreeMemory)
		if memTag == 0 {
			memTag = AVAIL_DATA
		}

		dirPage := tableObj.DirectoryPage

		dirPage.Mu.RLock()
		pageObj := dirPage.Value[space.PageID]
		dirPage.Mu.RUnlock()

		pageObj.Mu.Lock()
		if pageObj.Level != 0 {
			tableObj.Mu.Lock()
			rankSlice := tableObj.Memory[pageObj.Level]
			tableObj.Memory[pageObj.Level] = lo.Filter(rankSlice, func(item *FreeSpace, i int) bool {
				return space.PageID != item.PageID
			})
			tableObj.Mu.Unlock()
		}
		pageObj.Level = memTag
		pageObj.ExactFreeMem = space.FreeMemory
		pageObj.Mu.Unlock()

		// existing page already has something inside of the pointer array.
		err = UpdatePageInfo(newPage, tableObj, tableStats, nil, REPLACING)
		if err != nil {
			return fmt.Errorf("updatePageInfo failed: %w", err)
		}

		tableObj.Mu.Lock()
		tableObj.Memory[memTag] = append(tableObj.Memory[memTag], space)

		err = saveMemMapping(tableObj, tableStats)
		if err != nil {
			return fmt.Errorf("saveMemMapping failed: %w", err)
		}

		var freeSpaceType = reflect.TypeOf((*FreeSpace)(nil)).Elem()
		freed := accountingCtx.Release(freeSpaceType, space)
		if !freed {
			panic("freeSpaceObj wasn't freed")
		}

		tableObj.Mu.Unlock()

		if insertChan != nil {
			insertChan <- updateInfo.NonAddedRow
		}
	}

	logger.Log.Info("cleanOrgnize (end)")

	return nil
}

// ###[x] last two fields of pageObj aren't being saved on the updatePageInfo function but it gets add and saved here.
func memSeparationSingle(newSpace *FreeSpace, tableObj *TableObj, tableStats *TableInfo) error {
	memTag := getTag(newSpace.FreeMemory)

	dirPage := tableObj.DirectoryPage
	dirPage.Mu.Lock()
	defer dirPage.Mu.Unlock()

	pageObj := dirPage.Value[newSpace.PageID]
	pageObj.Mu.Lock()

	if memTag == 0 {
		memTag = AVAIL_DATA
	}

	tableObj.Mu.Lock()
	tableObj.Memory[memTag] = append(tableObj.Memory[memTag], newSpace)
	err := saveMemMapping(tableObj, tableStats)
	if err != nil {
		return fmt.Errorf("saveMemMapping failed: %w", err)
	}
	tableObj.Mu.Unlock()
	pageObj.Mu.Unlock()

	pageObj.Level = memTag
	pageObj.ExactFreeMem = newSpace.FreeMemory
	logger.Log.WithFields(logrus.Fields{"MemTag": memTag, "ExactFreeMem": pageObj.ExactFreeMem, "memLevel": pageObj.Level, "offset": pageObj.Offset}).Info("memory separation single done")

	err = UpdateDirectoryPageDisk(tableObj.DirectoryPage, tableObj.DirFile)
	if err != nil {
		return fmt.Errorf("UpdateDirectoryPageDisk failed: %w", err)
	}

	logger.Log.Info("Insertion Completed")
	return nil
}

func saveMemMapping(tableObj *TableObj, tableStats *TableInfo) error {
	tableStats.UsedSpace = AccountUsedMemory(tableObj.Memory)

	bytes, err := EncodeMemObj(tableObj.Memory)
	if err != nil {
		return fmt.Errorf("EncodeMemObj failed: %w", err)
	}

	err = WriteNonPageFile(tableObj.MemFile, bytes)
	if err != nil {
		return fmt.Errorf("WriteNonPageFile failed: %w", err)
	}

	return nil
}

func AccountUsedMemory(memory map[uint16][]*FreeSpace) uint64 {
	var totalMem uint64
	for k, v := range memory {
		rankMem := int(AVAIL_DATA-k) * len(v)
		totalMem += uint64(rankMem)
	}

	return totalMem
}

func searchPage(tableObj *TableObj, memoryNedded, level uint16) ([]*FreeSpace, *FreeSpace, uint16, int) {
	var memSlice []*FreeSpace
	var memTag uint16
	var spaceInfo *FreeSpace
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
			memSlice = []*FreeSpace{}
			level += NEXT_LEVEL
			continue
		}

	}

	return memSlice, spaceInfo, memTag, deleteIndex
}

func getAvailablePage(bufferM *BufferPoolManager, tableObj *TableObj, memoryNedded uint16, tableName string) (*PageV2, error) {
	var memSlice []*FreeSpace
	var memTag uint16
	var spaceInfo *FreeSpace
	var deleteIndex int

	memSlice, spaceInfo, memTag, deleteIndex = searchPage(tableObj, memoryNedded, memoryNedded)
	if memTag == 0 && spaceInfo == nil {
		logger.Log.Info("Created new page")

		page := CreatePageV2(tableName)

		return page, nil
	}

	// deleting page found
	memSlice = lo.Filter(memSlice, func(item *FreeSpace, i int) bool {
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
	case pageMem > FIRST_LEVEL && pageMem <= AVAIL_DATA:
		return AVAIL_DATA
	default:
		return 0
	}
}
