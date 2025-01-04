package storage

import (
	"a2gdb/logger"
	"errors"
	"log"
)

const (
	FetchPage   = "FETCH PAGE"
	InsertData  = "INSERT DATA"
	MaxPoolSize = 4000
)

type FrameID int
type BufferPoolManager struct {
	Pages       [MaxPoolSize]*PageV2
	freeList    []FrameID
	pageTable   map[PageID]FrameID
	Replacer    *LRUKReplacer
	DiskManager *DiskManagerV2
}

func (bpm *BufferPoolManager) InsertPage(page *PageV2) error {
	if len(bpm.freeList) == 0 {
		bpm.Evict()
	}

	frameID := bpm.freeList[0]
	bpm.freeList = bpm.freeList[1:]

	bpm.Pages[frameID] = page
	bpm.pageTable[PageID(page.Header.ID)] = frameID

	return nil
}

func (bpm *BufferPoolManager) FlushAll() {

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

	bpm.DeletePage(PageID(page.Header.ID))

	logger.Log.WithField("pageId", page.Header.ID).Info("PAGE EVICTED")
	return nil
}

func (bpm *BufferPoolManager) DeletePage(pageID PageID) (FrameID, error) {
	if frameID, ok := bpm.pageTable[pageID]; ok {
		delete(bpm.pageTable, pageID)
		bpm.Pages[frameID] = nil
		bpm.freeList = append(bpm.freeList, frameID)
		return frameID, nil
	}

	return 0, errors.New("page not found")
}

func (bpm *BufferPoolManager) FetchPage(pageID PageID) (*PageV2, error) {
	var pagePtr *PageV2

	if frameID, ok := bpm.pageTable[pageID]; ok {
		pagePtr = bpm.Pages[frameID]
		if pagePtr.IsPinned {
			return nil, errors.New("page is pinned, cannot access")
		}
	} else {

		// TODO # what if page not in memory

	}

	bpm.Pin(PageID(pagePtr.Header.ID))
	return pagePtr, nil
}

func (bpm *BufferPoolManager) Unpin(pageID PageID, isDirty bool) error {
	if FrameID, ok := bpm.pageTable[pageID]; ok {
		page := bpm.Pages[FrameID]
		page.IsDirty = isDirty
		page.IsPinned = false
		return nil
	}

	return errors.New("page not found")
}

func (bpm *BufferPoolManager) Pin(pageID PageID) error {
	if FrameID, ok := bpm.pageTable[pageID]; ok {
		page := bpm.Pages[FrameID]
		page.IsPinned = true
		bpm.Replacer.RecordAccess(FrameID)

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

	return &BufferPoolManager{pages, freeList, pageTable, replacer, diskManager}, nil
}
