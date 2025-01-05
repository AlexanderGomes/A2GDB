package storage

import (
	"a2gdb/logger"
	"errors"
	"log"
)

const (
	MaxPoolSize = 10
)

type FrameID int
type BufferPoolManager struct {
	Pages       [MaxPoolSize]*PageV2
	freeList    *[]FrameID
	PageTable   map[PageID]FrameID
	Replacer    *LRUKReplacer
	DiskManager *DiskManagerV2
}

func (bpm *BufferPoolManager) FullBufferScan() []*PageV2 {
	var pages []*PageV2
	for _, page := range bpm.Pages {
		if page == nil {
			continue
		}

		err := bpm.Pin(PageID(page.Header.ID))
		if err != nil {
			log.Fatal(err)
		}

		pages = append(pages, page)
	}

	return pages
}

func (bpm *BufferPoolManager) ReplacePage(page *PageV2) {
	if frameID, ok := bpm.PageTable[PageID(page.Header.ID)]; ok {
		bpm.Pages[frameID] = page
	}
}

func (bpm *BufferPoolManager) InsertPage(page *PageV2) {
	logger.Log.Info("Into BPM, pageID: ", page.Header.ID)

	if len(*bpm.freeList) == 0 {
		bpm.Evict()
	}

	logger.Log.Info("free list size: ", len(*bpm.freeList))

	frameID := (*bpm.freeList)[0]
	*bpm.freeList = (*bpm.freeList)[1:]

	bpm.Pages[frameID] = page
	bpm.PageTable[PageID(page.Header.ID)] = frameID
}

func (bpm *BufferPoolManager) Evict() error {
	frameID, err := bpm.Replacer.Evict()
	if err != nil {
		return err
	}

	page := bpm.Pages[frameID]

	//## disk
	tableObj := bpm.DiskManager.TableObjs[page.TABLE]
	err = UpdatePageInfo(nil, page, tableObj)
	if err != nil {
		log.Fatal(err)
	}

	err = bpm.DeletePage(PageID(page.Header.ID))
	if err != nil {
		log.Fatal(err)
	}

	logger.Log.WithField("pageId", page.Header.ID).Info("PAGE EVICTED")
	return nil
}

func (bpm *BufferPoolManager) DeletePage(pageID PageID) error {
	if frameID, ok := bpm.PageTable[pageID]; ok {
		delete(bpm.PageTable, pageID)
		bpm.Pages[frameID] = nil
		*bpm.freeList = append(*bpm.freeList, frameID)
		logger.Log.WithField("freeList", bpm.freeList).Info("Frame Re-Added")
		return nil
	}

	return errors.New("page not found")
}

func (bpm *BufferPoolManager) FetchPage(pageID PageID, tableObj *TableObj) (*PageV2, error) {

	var pagePtr *PageV2
	if frameID, ok := bpm.PageTable[pageID]; ok {
		logger.Log.Info("Fetching from BPM, pageId: ", pageID)
		pagePtr = bpm.Pages[frameID]
		if pagePtr.IsPinned {
			return nil, errors.New("page is pinned, cannot access")
		}
	} else {
		logger.Log.Info("Fetching from Disk, pageId: ", pageID)
		pageInfo := tableObj.DirectoryPage.Value[pageID]
		pageBytes, err := ReadPageAtOffset(tableObj.DataFile, pageInfo.Offset)
		if err != nil {
			log.Panicf("reading page at offset %s failed", err)
		}

		pagePtr, err = DecodePageV2(pageBytes)
		if err != nil {
			log.Panicf("decoding page from offset %s failed", err)
		}

		bpm.InsertPage(pagePtr)
	}

	bpm.Pin(PageID(pagePtr.Header.ID))
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

	return errors.New("page not found")
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
