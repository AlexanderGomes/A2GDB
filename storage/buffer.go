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
	Data      []Row
}

type BufferRes struct {
	PageID  PageID
	PagePtr *Page
	Error   error
}

type FrameID int
type BufferPoolManager struct {
	Pages         [MaxPoolSize]*Page
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
		bpm.DiskScheduler.DiskManager.WriteToDisk(req)
	}
}

func (bpm *BufferPoolManager) InsertPage(page *Page) error {
	if len(bpm.freeList) == 0 {
		for i := 0; i < 20; i++ {
			bpm.Evict()
		}
	}
	frameID := bpm.freeList[0]
	bpm.freeList = bpm.freeList[1:]

	bpm.Pages[frameID] = page
	bpm.pageTable[page.ID] = frameID

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
	bpm.DeletePage(page.ID)

	fmt.Println("PAGE EVICTED:", page.ID)
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


func (bpm *BufferPoolManager) FetchPage(pageID PageID) (*Page, error) {
	var pagePtr *Page

	if frameID, ok := bpm.pageTable[pageID]; ok {
		pagePtr = bpm.Pages[frameID]
		if pagePtr.IsPinned {
			return nil, errors.New("Page is pinned, cannot access")
		}
	} else {

		// change
		page := Page{
			ID: pageID,
		}

		// change
		req := DiskReq{
			Page:      page,
			Operation: "READ",
		}

		bpm.DiskScheduler.AddReq(req)
		for result := range bpm.DiskScheduler.ResultChan {
			if result.Response != nil {
				return nil, result.Response
			}

			if result.Page.ID == pageID {
				bpm.InsertPage(&result.Page)
				bpm.Pin(result.Page.ID)
				pagePtr = &result.Page
				break
			}
		}
	}

	bpm.Pin(pagePtr.ID)
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
	pages := [MaxPoolSize]*Page{}
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
