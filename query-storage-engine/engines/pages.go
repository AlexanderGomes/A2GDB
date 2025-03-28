package engines

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"os"
	"sync"
)

const (
	PageSizeV2   = 4 * 1024
	HeaderSize   = 14
	PageDataSize = PageSizeV2 - HeaderSize
	ADDING       = 0
	REPLACING    = 1
)

type RowV2 struct {
	ID     uint64
	Values map[string]string
	Size   uint64
}

type PageID uint64
type PageHeader struct {
	ID        uint64
	LowerPtr  uint16
	UpperPtr  uint16
	NumTuples uint16
}

type PageV2 struct {
	Header       PageHeader
	TABLE        string
	PointerArray []TupleLocation
	Data         []byte
	IsPinned     bool
	IsDirty      bool
}

type TupleLocation struct {
	Offset uint16
	Length uint16
	Free   bool
}

type DirectoryPageV2 struct {
	Value map[PageID]*PageInfo
	Mu    sync.RWMutex
}

type Offset uint64
type PageInfo struct {
	Offset       Offset
	PointerArray []TupleLocation
	Level        uint16
	ExactFreeMem uint16
	Mu           sync.RWMutex
}

func CreatePageV2(tableName string) *PageV2 {
	return &PageV2{
		Header: PageHeader{
			ID:        GenerateRandomID(),
			LowerPtr:  uint16(HeaderSize),
			UpperPtr:  uint16(PageDataSize),
			NumTuples: 0,
		},
		TABLE:        tableName,
		PointerArray: []TupleLocation{},
		Data:         make([]byte, PageDataSize),
	}
}

func (p *PageV2) AddTuple(data []byte, verbose string) error {
	tupleLen := uint16(len(data))
	offset := p.Header.UpperPtr - tupleLen
	canInsert := p.Header.UpperPtr-p.Header.LowerPtr > tupleLen && offset < PageDataSize

	if !canInsert {
		fmt.Println("(addTuple) pageHeader: ", p.Header)
		fmt.Println(verbose)
		return fmt.Errorf("AddTuple (can't insert)")
	}

	copy(p.Data[offset:], data)

	tupleLocation := TupleLocation{
		Offset: offset,
		Length: tupleLen,
	}

	p.PointerArray = append(p.PointerArray, tupleLocation)
	p.Header.NumTuples++
	p.Header.UpperPtr -= tupleLen
	return nil
}

func WritePageBackV2(page *PageV2, offset Offset, tableDataFile *os.File) error {
	pageBytes, err := EncodePageV2(page)
	if err != nil {
		return fmt.Errorf("EncodePageV2 failed: %w", err)
	}

	_, err = tableDataFile.WriteAt(pageBytes, int64(offset))
	if err != nil {
		return fmt.Errorf("dataFile.WriteAt failed: %w", err)
	}

	return nil
}

func WritePageEOFV2(page *PageV2, dataFile *os.File) (Offset, error) {
	pageBytes, err := EncodePageV2(page)
	if err != nil {
		return 0, fmt.Errorf("EncodePageV2 failed: %w", err)
	}

	fileInfo, err := dataFile.Stat()
	if err != nil {
		return 0, fmt.Errorf("dataFile.Stat(failed): %w", err)
	}

	offset := fileInfo.Size()
	_, err = dataFile.WriteAt(pageBytes, offset)
	if err != nil {
		return 0, fmt.Errorf("dataFile.WriteAt(failed): %w", err)
	}

	return Offset(offset), nil
}

func ReadNonPageFile(file *os.File) ([]byte, error) {
	var buffer bytes.Buffer
	tempBuffer := make([]byte, 1024)

	for {
		n, err := file.Read(tempBuffer)
		if n > 0 {
			buffer.Write(tempBuffer[:n])
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("reading from file (failed): %w", err)
		}
	}

	return buffer.Bytes(), nil
}

func WriteNonPageFile(file *os.File, data []byte) error {
	if file == nil {
		return fmt.Errorf("nil file pointer")
	}

	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("truncating file failed: %w", err)
	}

	_, err := file.WriteAt(data, 0)
	if err != nil {
		return fmt.Errorf("writing to file (failed): %w", err)
	}

	return nil
}

func ReadPageAtOffset(file *os.File, offset Offset) ([]byte, error) {
	pageData := make([]byte, PageSizeV2)
	_, err := file.ReadAt(pageData, int64(offset))
	if err != nil {
		return nil, fmt.Errorf("reading from file (failed): %w", err)
	}

	return pageData, nil
}

func (dm *DiskManagerV2) UpdateCatalog() error {
	bytes, err := SerializeCatalog(dm.PageCatalog)
	if err != nil {
		return fmt.Errorf("SerializeCatalog failed: %w", err)
	}

	_, err = dm.FileCatalog.WriteAt(bytes, 0)
	if err != nil {
		return fmt.Errorf("writing to file (failed): %w", err)
	}

	return nil
}

func GenerateRandomID() uint64 {
	max := new(big.Int).Lsh(big.NewInt(1), 64)
	randomNum, _ := rand.Int(rand.Reader, max)

	return randomNum.Uint64()
}

func RearrangePAGE(page *PageV2, tableObj *TableObj, tableName string) (*PageV2, error) {
	newPage := CreatePageV2(tableName)
	newPage.Header.ID = page.Header.ID
	newPage.IsPinned = page.IsPinned

	directoryPage := tableObj.DirectoryPage

	directoryPage.Mu.RLock()
	pageObj, ok := tableObj.DirectoryPage.Value[PageID(page.Header.ID)]
	if !ok {
		return nil, fmt.Errorf("pageObj not found")
	}
	directoryPage.Mu.RUnlock()

	pageObj.Mu.Lock()
	defer pageObj.Mu.Unlock()

	for i := range pageObj.PointerArray {
		location := &pageObj.PointerArray[i]

		if !location.Free {
			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			err := newPage.AddTuple(rowBytes, "RearrangePAGE")
			if err != nil {
				return nil, fmt.Errorf("AddTuple failed: %w", err)
			}
			continue
		}
	}

	return newPage, nil
}

func UpdatePageInfo(pageFound *PageV2, tableObj *TableObj, tableStats *TableInfo, manager *DiskManagerV2, operation int) error {
	pageID := PageID(pageFound.Header.ID)

	dirPage := tableObj.DirectoryPage
	dirPage.Mu.Lock()
	defer dirPage.Mu.Unlock()

	pageObj, found := dirPage.Value[pageID]
	if !found {
		offset, err := WritePageEOFV2(pageFound, tableObj.DataFile)
		if err != nil {
			return fmt.Errorf("WritePageEOFV2 failed: %w", err)
		}

		pageObj = &PageInfo{
			Offset:       offset,
			PointerArray: pageFound.PointerArray,
			ExactFreeMem: pageFound.Header.UpperPtr - pageFound.Header.LowerPtr,
		}

		dirPage.Value[pageID] = pageObj

		tableStats.NumOfPages++

		err = manager.UpdateCatalog()
		if err != nil {
			return fmt.Errorf("UpdateCatalog Failed: %w", err)
		}
	} else {
		pageObj.Mu.Lock()
		if operation == ADDING {
			pageObj.PointerArray = append(pageObj.PointerArray, pageFound.PointerArray...)
		} else if operation == REPLACING {
			pageObj.PointerArray = pageFound.PointerArray
		}
		pageObj.Mu.Unlock()

		if err := WritePageBackV2(pageFound, pageObj.Offset, tableObj.DataFile); err != nil {
			return fmt.Errorf("WritePageBackV2 failed: %w", err)
		}
	}

	pageFound.PointerArray = []TupleLocation{} // in memory should be clear

	if err := UpdateDirectoryPageDisk(dirPage, tableObj.DirFile); err != nil {
		return fmt.Errorf("UpdateDirectoryPageDisk failed: %w", err)
	}

	return nil
}
