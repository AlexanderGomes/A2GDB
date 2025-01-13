package storage

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"sync"
)

const (
	PageSizeV2 = 4 * 1024
	HeaderSize = 14

	PageDataSize   = PageSizeV2 - HeaderSize
	TupleEntrySize = 4
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

func (p *PageV2) AddTuple(data []byte) error {
	tupleLen := uint16(len(data))
	offset := p.Header.UpperPtr - tupleLen
	canInsert := p.Header.UpperPtr-p.Header.LowerPtr > tupleLen && offset < PageDataSize

	if !canInsert {
		return fmt.Errorf("AddTuple (can't insert)")
	}

	copy(p.Data[offset:], data)

	tupleLocation := TupleLocation{
		Offset: offset,
		Length: tupleLen,
		Free:   false,
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

func GenerateRandomIDRow() int64 {
	max := new(big.Int).Lsh(big.NewInt(1), 64)
	randomNum, _ := rand.Int(rand.Reader, max)

	return randomNum.Int64()
}

func RearrangePAGE(page *PageV2, tableObj *TableObj, tableName string) (*PageV2, error) {
	newPage := CreatePageV2(tableName)
	newPage.Header.ID = page.Header.ID
	newPage.IsPinned = page.IsPinned

	pageObj, ok := tableObj.DirectoryPage.Value[PageID(page.Header.ID)]
	if !ok {
		return nil, fmt.Errorf("pageObj not found")
	}

	for _, location := range pageObj.PointerArray {
		if !location.Free {
			rowBytes := page.Data[location.Offset : location.Offset+location.Length]

			err := newPage.AddTuple(rowBytes)
			if err != nil {
				return nil, fmt.Errorf("AddTuple failed: %w", err)
			}
		}
	}

	pageObj.PointerArray = newPage.PointerArray

	return newPage, nil
}

func UpdatePageInfo(rowsID []uint64, pageFound *PageV2, tableObj *TableObj, tableStats *TableInfo) error {
	pageID := PageID(pageFound.Header.ID)
	dirPage := tableObj.DirectoryPage
	pageInfObj, found := dirPage.Value[pageID]

	if !found {
		offset, err := WritePageEOFV2(pageFound, tableObj.DataFile)
		if err != nil {
			return fmt.Errorf("WritePageEOFV2 failed: %w", err)
		}

		pageInfObj = &PageInfo{
			Offset:       offset,
			PointerArray: pageFound.PointerArray,
		}

		dirPage.Value[pageID] = pageInfObj
		tableStats.NumOfPages++
	} else {
		pageInfObj.PointerArray = append(pageInfObj.PointerArray, pageFound.PointerArray...)
		if err := WritePageBackV2(pageFound, pageInfObj.Offset, tableObj.DataFile); err != nil {
			return fmt.Errorf("WritePageBackV2 failed: %w", err)
		}
	}

	pageFound.PointerArray = []TupleLocation{} // in memory should be clear

	if err := UpdateDirectoryPageDisk(dirPage, tableObj.DirFile); err != nil {
		return fmt.Errorf("UpdateDirectoryPageDisk failed: %w", err)
	}

	if err := UpdateBp(rowsID, tableObj, *pageInfObj); err != nil { // race chain // cleanOrganize // handleLikeInsert
		return fmt.Errorf("UpdateBp failed: %w", err)
	}

	return nil
}

func GetAllRows(tableName string, manager *DiskManagerV2) ([]*RowV2, error) {
	var rows []*RowV2

	tableObj, err := GetTableObj(tableName, manager)
	if err != nil {
		return nil, fmt.Errorf("GetTableObj failed: %w", err)
	}

	directoryMap := tableObj.DirectoryPage.Value
	pages, err := GetTablePagesFromDiskTest(tableObj.DataFile)
	if err != nil {
		return nil, fmt.Errorf("GetTablePagesFromDisk failed: %w", err)
	}

	for _, page := range pages {
		pageId := PageID(page.Header.ID)
		pageObj, ok := directoryMap[pageId]
		if !ok {
			return nil, errors.New("pageObj not found")
		}

		for _, location := range pageObj.PointerArray {
			if location.Free {
				continue
			}

			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := DecodeRow(rowBytes)
			if err != nil {
				return nil, fmt.Errorf("DecodeRow failed: %w", err)
			}

			rows = append(rows, row)
		}
	}

	return rows, nil
}
