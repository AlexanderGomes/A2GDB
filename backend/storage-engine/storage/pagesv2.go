package storage

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
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
	TABLE        TableName
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
}

type Offset uint64
type PageInfo struct {
	Offset       Offset
	PointerArray []TupleLocation
	Level        uint16
	ExactFreeMem uint16
}

func CreatePageV2() *PageV2 {
	return &PageV2{
		Header: PageHeader{
			ID:        GenerateRandomID(),
			LowerPtr:  uint16(HeaderSize),
			UpperPtr:  uint16(PageDataSize),
			NumTuples: 0,
		},
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
		return fmt.Errorf("WritePageBack: %w", err)
	}

	_, err = tableDataFile.WriteAt(pageBytes, int64(offset))
	if err != nil {
		return fmt.Errorf("WritePageBack (failed writing page to disk): %w", err)
	}

	return nil
}

func WritePageEOFV2(page *PageV2, dataFile *os.File) (Offset, error) {
	pageBytes, err := EncodePageV2(page)
	if err != nil {
		return 0, fmt.Errorf("WritePageEOF: %w", err)
	}

	fileInfo, err := dataFile.Stat()
	if err != nil {
		return 0, fmt.Errorf("WritePageEOF (getting file info): %w", err)
	}

	offset := fileInfo.Size()
	_, err = dataFile.WriteAt(pageBytes, offset)
	if err != nil {
		return 0, fmt.Errorf("WritePageEOF (writing file to disk): %w", err)
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
			return nil, fmt.Errorf("ReadNonPageFile: error reading file: %w", err)
		}
	}

	return buffer.Bytes(), nil
}

func WriteNonPageFile(file *os.File, data []byte) error {
	if file == nil {
		return fmt.Errorf("WriteNonPageFile (file pointer is nil)")
	}

	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("WriteNonPageFile (error truncating file): %w", err)
	}

	_, err := file.WriteAt(data, 0)
	if err != nil {
		return fmt.Errorf("WriteNonPageFile (error writing to file): %w", err)
	}

	return nil
}

func ReadPageAtOffset(file *os.File, offset Offset) ([]byte, error) {
	pageData := make([]byte, PageSizeV2)
	_, err := file.ReadAt(pageData, int64(offset))
	if err != nil {
		return nil, fmt.Errorf("failed to read page data: %w", err)
	}

	return pageData, nil
}

func (dm *DiskManagerV2) UpdateCatalog() error {
	bytes, err := SerializeCatalog(dm.PageCatalog)

	if err != nil {
		return fmt.Errorf("UpdateCatalog: %w", err)
	}

	dm.FileCatalog.WriteAt(bytes, 0)

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

func RearrangePAGE(page *PageV2, tableObj *TableObj) (*PageV2, error) {
	newPage := CreatePageV2()
	newPage.Header.ID = page.Header.ID

	pageObj, ok := tableObj.DirectoryPage.Value[PageID(page.Header.ID)]
	if !ok {
		return nil, fmt.Errorf("RearrangePAGE: pageObj not found")
	}

	for _, location := range pageObj.PointerArray {
		if !location.Free {
			rowBytes := page.Data[location.Offset : location.Offset+location.Length]

			err := newPage.AddTuple(rowBytes)
			if err != nil {
				return nil, fmt.Errorf("RearrangePAGE: %w", err)
			}
		}
	}

	pageObj.PointerArray = newPage.PointerArray

	return newPage, nil
}

func UpdatePageInfo(rowsID []uint64, pageFound *PageV2, tableObj *TableObj) error {
	pageID := PageID(pageFound.Header.ID)
	dirPage := tableObj.DirectoryPage
	pageInfObj, found := dirPage.Value[pageID]

	if !found {
		offset, err := WritePageEOFV2(pageFound, tableObj.DataFile)
		if err != nil {
			return fmt.Errorf("write page EOF error: %w", err)
		}

		pageInfObj = &PageInfo{
			Offset:       offset,
			PointerArray: pageFound.PointerArray,
		}

		dirPage.Value[pageID] = pageInfObj
	} else {
		pageInfObj.PointerArray = append(pageInfObj.PointerArray, pageFound.PointerArray...)
		if err := WritePageBackV2(pageFound, pageInfObj.Offset, tableObj.DataFile); err != nil {
			return fmt.Errorf("write page back error: %w", err)
		}
	}

	if err := UpdateDirectoryPageDisk(dirPage, tableObj.DirFile); err != nil {
		return fmt.Errorf("update directory page error: %w", err)
	}

	if err := UpdateBp(rowsID, *tableObj, *pageInfObj); err != nil {
		return fmt.Errorf("update B+ tree error: %w", err)
	}

	return nil
}

func GetAllRows(tableName string, manager *DiskManagerV2) []*RowV2 {
	var rows []*RowV2

	tableObj, err := GetTableObj(tableName, manager)
	if err != nil {
		log.Fatalf("GetTable failed for: %s, error: %s", tableName, err)
	}

	directoryMap := tableObj.DirectoryPage.Value
	pages, err := GetTablePages(tableObj.DataFile, nil)
	if err != nil {
		log.Fatalf("GetTablePages failed for: %s, error: %s", tableName, err)
	}

	for _, page := range pages {
		pageId := PageID(page.Header.ID)
		pageObj, ok := directoryMap[pageId]

		if !ok {
			log.Fatalf("PageObj not found for page: %v", page.Header.ID)
		}

		for _, location := range pageObj.PointerArray {
			if location.Free {
				continue
			}

			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := DecodeRow(rowBytes)
			if err != nil {
				log.Fatalf("DecodeRow error: %s", err)
			}

			rows = append(rows, row)
		}
	}

	return rows
}
