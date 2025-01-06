package storage

import (
	"a2gdb/logger"
	"errors"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
)

const (
	MaxPoolSize = 100
)

type FrameID int
type BufferPoolManager struct {
	Pages       [MaxPoolSize]*PageV2
	freeList    *[]FrameID
	PageTable   map[PageID]FrameID
	Replacer    *LRUKReplacer
	DiskManager *DiskManagerV2
}

func (bpm *BufferPoolManager) FullTableScan(dataFile *os.File, pageTable map[PageID]FrameID) ([]*PageV2, error) {
	var mergedPages []*PageV2

	bufferPages, err := bpm.FullBufferScan()
	if err != nil {
		return nil, fmt.Errorf("FullBufferScan failed: %w", err)
	}

	diskPages, err := GetTablePagesFromDisk(dataFile, nil, pageTable)
	if err != nil {
		return nil, err
	}

	logger.Log.WithFields(logrus.Fields{"bufferPages": len(bufferPages), "diskPages": len(diskPages)}).Info("Scanning Disk && Buffer Pool")

	mergedPages = append(mergedPages, diskPages...)
	mergedPages = append(mergedPages, bufferPages...)

	return mergedPages, nil
}

func (bpm *BufferPoolManager) FullBufferScan() ([]*PageV2, error) {
	var pages []*PageV2
	for _, page := range bpm.Pages {
		if page == nil {
			continue
		}

		err := bpm.Pin(PageID(page.Header.ID))
		if err != nil {
			return nil, fmt.Errorf("Pin Failed: %w", err)
		}

		pages = append(pages, page)
	}

	return pages, nil
}

// ## when rearranging a new page is being created, so this is necessary
func (bpm *BufferPoolManager) ReplacePage(page *PageV2) error {
	if frameID, ok := bpm.PageTable[PageID(page.Header.ID)]; ok {
		bpm.Pages[frameID] = page
		return nil
	}

	return fmt.Errorf("pageId %d not in memory", page.Header.ID)
}

func (bpm *BufferPoolManager) InsertPage(page *PageV2) error {
	logger.Log.Info("Insert Into BPM, pageID: ", page.Header.ID)

	if len(*bpm.freeList) == 0 {
		err := bpm.Evict()
		if err != nil {
			return fmt.Errorf("bpm.Evict failed: %w", err)
		}
	}

	frameID := (*bpm.freeList)[0]
	*bpm.freeList = (*bpm.freeList)[1:]

	bpm.Pages[frameID] = page
	bpm.PageTable[PageID(page.Header.ID)] = frameID

	logger.Log.Info("Free List Size After Insert: ", len(*bpm.freeList))
	return nil
}

func (bpm *BufferPoolManager) Evict() error {
	frameID, err := bpm.Replacer.Evict()
	if err != nil {
		return fmt.Errorf("Replacer.Evict failed: %w", err)
	}

	page := bpm.Pages[frameID]

	//## disk
	tableObj := bpm.DiskManager.TableObjs[page.TABLE]
	err = UpdatePageInfo(nil, page, tableObj)
	if err != nil {
		return fmt.Errorf("UpdatePageInfo failed: %w", err)
	}

	err = bpm.DeletePage(PageID(page.Header.ID))
	if err != nil {
		return fmt.Errorf("DeletePage failed: %w", err)
	}

	logger.Log.WithFields(logrus.Fields{"PageId": page.Header.ID, "FrameId": frameID}).Info("PAGE EVICTED")
	return nil
}

func (bpm *BufferPoolManager) DeletePage(pageID PageID) error {
	if frameID, ok := bpm.PageTable[pageID]; ok {
		delete(bpm.PageTable, pageID)
		bpm.Pages[frameID] = nil
		*bpm.freeList = append(*bpm.freeList, frameID)
		return nil
	}

	return errors.New("page not found")
}

func (bpm *BufferPoolManager) FetchPage(pageID PageID, tableObj *TableObj) (*PageV2, error) {
	var pagePtr *PageV2
	if frameID, ok := bpm.PageTable[pageID]; ok {
		pagePtr = bpm.Pages[frameID]
		if pagePtr.IsPinned {
			return nil, errors.New("page is pinned, cannot access")
		}
		logger.Log.Info("Fetched from BPM, pageId: ", pageID)
	} else {
		pageInfo := tableObj.DirectoryPage.Value[pageID]
		pageBytes, err := ReadPageAtOffset(tableObj.DataFile, pageInfo.Offset)
		if err != nil {
			return nil, fmt.Errorf("ReadPageAtOffset failed: %w", err)
		}

		pagePtr, err = DecodePageV2(pageBytes)
		if err != nil {
			return nil, fmt.Errorf("DecodePageV2 failed: %w", err)
		}

		err = bpm.InsertPage(pagePtr)
		if err != nil {
			return nil, fmt.Errorf("InsertPage failed: %w", err)
		}
		logger.Log.Info("Fetched from Disk, pageId: ", pageID)
	}

	err := bpm.Pin(PageID(pagePtr.Header.ID))
	if err != nil {
		return nil, fmt.Errorf("Pin failed: %w", err)
	}

	return pagePtr, nil
}

func (bpm *BufferPoolManager) Unpin(pageID PageID, isDirty bool) error {
	if FrameID, ok := bpm.PageTable[pageID]; ok {
		page := bpm.Pages[FrameID]
		page.IsDirty = isDirty
		page.IsPinned = false
		logger.Log.Info("Unpinned pageId: ", pageID)
		return nil
	}

	return fmt.Errorf("pageId: %d not in memory", pageID)
}

func (bpm *BufferPoolManager) Pin(pageID PageID) error {
	if FrameID, ok := bpm.PageTable[pageID]; ok {
		page := bpm.Pages[FrameID]
		page.IsPinned = true
		bpm.Replacer.RecordAccess(FrameID)

		logger.Log.Info("Pinned PageId: ", pageID)
		return nil
	}

	return errors.New("page not found")
}

func NewBufferPoolManager(k int, fileName string) (*BufferPoolManager, error) {
	freeList := make([]FrameID, 0)
	pages := [MaxPoolSize]*PageV2{}
	for i := 0; i < MaxPoolSize; i++ {
		freeList = append(freeList, FrameID(i))
		pages[FrameID(i)] = nil
	}
	pageTable := make(map[PageID]FrameID)

	replacer := NewLRUKReplacer(k)
	diskManager, err := NewDiskManagerV2(fileName)
	if err != nil {
		return nil, err
	}

	return &BufferPoolManager{pages, &freeList, pageTable, replacer, diskManager}, nil
}
