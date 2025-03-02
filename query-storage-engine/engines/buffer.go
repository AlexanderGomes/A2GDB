package engines

import (
	"a2gdb/logger"
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
)

const (
	MaxPoolSize = 30
)

type FrameID int
type BufferPoolManager struct {
	Pages       [MaxPoolSize]*PageV2
	freeList    *[]FrameID
	PageTable   map[PageID]FrameID
	Replacer    *LRUKReplacer
	DiskManager *DiskManagerV2
	Wal         *WalManager
	Mu          sync.RWMutex
}

func (bpm *BufferPoolManager) FullTableScan(ctx context.Context, pageChan chan *PageV2, tableObj *TableObj, staticNumPages uint64) error {
	var wg sync.WaitGroup

	errChan := make(chan error, 2)

	defer close(pageChan)
	defer close(errChan)

	wg.Add(2)
	go func() {
		defer wg.Done()
		// not receiving the context cancelation
		if err := bpm.FullBufferScan(pageChan); err != nil {
			errChan <- fmt.Errorf("FullBufferScan Failed: %w", err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := GetTablePagesFromDisk(ctx, nil, pageChan, tableObj, bpm.PageTable, staticNumPages); err != nil {
			errChan <- fmt.Errorf("GetTablePagesFromDisk Failed: %w", err)
		}
	}()

	wg.Wait()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (bpm *BufferPoolManager) FullBufferScan(pageChan chan *PageV2) error {
	for _, page := range bpm.Pages {
		if page == nil {
			continue
		}

		err := bpm.Pin(PageID(page.Header.ID))
		if err != nil {
			return fmt.Errorf("Pin Failed: %w", err)
		}

		logger.Log.WithField("pageID", page.Header.ID).Info("Page From Bpm")
		pageChan <- page
	}

	return nil
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
	if _, ok := bpm.PageTable[PageID(page.Header.ID)]; ok {
		return nil
	}

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

// stop evicting pinned pages
func (bpm *BufferPoolManager) Evict() error {
	frameID, err := bpm.Replacer.Evict()
	if err != nil {
		return fmt.Errorf("Replacer.Evict failed: %w", err)
	}

	page := bpm.Pages[frameID]

	for page.IsPinned {
		bpm.Replacer.RecordAccess(frameID, 1000)

		frameID, err = bpm.Replacer.Evict()
		if err != nil {
			return fmt.Errorf("Replacer.Evict failed: %w", err)
		}

		page = bpm.Pages[frameID]
	}

	logger.Log.WithField("pageId", page.Header.ID).Info("Found Non Pinned Page")

	//## disk
	tableObj := bpm.DiskManager.TableObjs[page.TABLE]
	tableStats := bpm.DiskManager.PageCatalog.Tables[tableObj.TableName]

	err = UpdatePageInfo(page, tableObj, tableStats, bpm.DiskManager, ADDING)
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
		tableObj.DirectoryPage.Mu.RLock()
		pageObj := tableObj.DirectoryPage.Value[pageID]
		tableObj.DirectoryPage.Mu.RUnlock()

		pageObj.Mu.RLock()
		defer pageObj.Mu.RUnlock()

		pageBytes, err := ReadPageAtOffset(tableObj.DataFile, pageObj.Offset)
		if err != nil {
			return nil, fmt.Errorf("ReadPageAtOffset failed: %w", err)
		}

		pagePtr, err = DecodePageV2(pageBytes)
		if err != nil {
			return nil, fmt.Errorf("DecodePageV2 failed: %w", err)
		}
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
		bpm.Replacer.RecordAccess(FrameID, 0)

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

	wal, err := NewWalManager(fileName + "/wal_logs")
	if err != nil {
		return nil, fmt.Errorf("NewWalManager failed initialization: %w", err)
	}

	return &BufferPoolManager{pages, &freeList, pageTable, replacer, diskManager, wal, sync.RWMutex{}}, nil
}
