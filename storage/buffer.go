package storage

import (
	"errors"
	"fmt"
)

const (
	FetchPage   = "FETCH PAGE"
	InsertData  = "INSERT DATA"
	MaxPoolSize = 4000
)

type BufferReq struct {
	Operation string
	PageID    PageID
	Data      []RowV2
}

type BufferRes struct {
	PageID  PageID
	PagePtr *PageV2
	Error   error
}

type FrameID int
type BufferPoolManager struct {
	Pages         [MaxPoolSize]*PageV2
	freeList      []FrameID
	pageTable     map[PageID]FrameID
	Replacer      *LRUKReplacer
	DiskScheduler *DiskScheduler
}

func (bpm *BufferPoolManager) FlushAll() {
	for _, FrameID := range bpm.pageTable {
		page := bpm.Pages[FrameID]
		req := DiskReq{
			Page:      *page,
			Operation: "WRITE",
		}
		bpm.DiskScheduler.AddReq(req)
	}
}

func (bpm *BufferPoolManager) InsertPage(page *PageV2) error {
	if len(bpm.freeList) == 0 {
		for i := 0; i < 20; i++ {
			bpm.Evict()
		}
	}
	frameID := bpm.freeList[0]
	bpm.freeList = bpm.freeList[1:]

	bpm.Pages[frameID] = page
	bpm.pageTable[PageID(page.Header.ID)] = frameID

	return nil
}

func (bpm *BufferPoolManager) Evict() error {
	frameID, err := bpm.Replacer.Evict()
	if err != nil {
		return err
	}
	page := bpm.Pages[frameID]

	req := DiskReq{
		Page:      *page,
		Operation: "WRITE",
	}

	bpm.DiskScheduler.AddReq(req)
	bpm.DeletePage(PageID(page.Header.ID))

	fmt.Println("PAGE EVICTED:", page.Header.ID)
	return nil
}

func (bpm *BufferPoolManager) DeletePage(pageID PageID) (FrameID, error) {
	if frameID, ok := bpm.pageTable[pageID]; ok {
		delete(bpm.pageTable, pageID)
		bpm.Pages[frameID] = nil
		bpm.freeList = append(bpm.freeList, frameID)
		return frameID, nil
	}
	return 0, errors.New("Page not found")
}

func (bpm *BufferPoolManager) FetchPage(pageID PageID) (*PageV2, error) {
	var pagePtr *PageV2

	if frameID, ok := bpm.pageTable[pageID]; ok {
		pagePtr = bpm.Pages[frameID]
		if pagePtr.IsPinned {
			return nil, errors.New("Page is pinned, cannot access")
		}
	} else {

		// TODO # Why create a page just to pass the ID ?
		page := PageV2{}
		page.Header.ID = uint64(pageID)

		req := DiskReq{
			Page:      page,
			Operation: "READ",
		}

		bpm.DiskScheduler.AddReq(req)
		for result := range bpm.DiskScheduler.ResultChan {
			if result.Response != nil {
				return nil, result.Response
			}

			if PageID(result.Page.Header.ID) == pageID {
				bpm.InsertPage(&result.Page)
				bpm.Pin(PageID(result.Page.Header.ID))
				pagePtr = &result.Page
				break
			}
		}
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

	return errors.New("Page Not Found")
}

func (bpm *BufferPoolManager) Pin(pageID PageID) error {
	if FrameID, ok := bpm.pageTable[pageID]; ok {
		page := bpm.Pages[FrameID]
		page.IsPinned = true
		bpm.Replacer.RecordAccess(FrameID)

		return nil
	}

	return errors.New("Page Not Found")
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
	DiskScheduler := NewDiskScheduler(diskManager)
	if err != nil {
		return nil, err
	}

	return &BufferPoolManager{pages, freeList, pageTable, replacer, DiskScheduler}, nil
}
