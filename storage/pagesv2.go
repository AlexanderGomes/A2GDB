package storage

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

const (
	PageSizeV2     = 4 * 1024
	HeaderSize     = 14
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
}

type DirectoryPageV2 struct {
	Value map[PageID]*PageInfo
}

type Offset uint64
type PageInfo struct {
	Offset       Offset
	PointerArray []TupleLocation
}

func CreatePageV2() *PageV2 {
	return &PageV2{
		Header: PageHeader{
			ID:        generateRandomID(),
			LowerPtr:  uint16(HeaderSize + 1),
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
	canInsert := p.Header.UpperPtr-p.Header.LowerPtr > tupleLen

	if !canInsert {
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

func (dm *DiskManagerV2) WritePageBackV2(page *PageV2, offset Offset, tableDataFile *os.File) error {
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

func (ds *DiskManagerV2) WritePageEOFV2(page *PageV2, dataFile *os.File) (Offset, error) {
	pageBytes, err := EncodePageV2(page)
	if err != nil {
		return 0, fmt.Errorf("WritePageEOF: %w", err)
	}

	fileInfo, err := dataFile.Stat()
	if err != nil {
		return 0, fmt.Errorf("WritePageEOF (getting file info): %w", err)
	}

	offset := fileInfo.Size()
	_, err = dataFile.Write(pageBytes)
	if err != nil {
		return 0, fmt.Errorf("WritePageEOF (writing file to disk): %w", err)
	}

	return Offset(offset), nil
}

func ReadDirFileV2(dirFile *os.File) ([]byte, error) {
	var buffer bytes.Buffer

	tempBuffer := make([]byte, 1024)

	for {
		n, err := dirFile.Read(tempBuffer)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("ReadFile (error reading directory file): %w", err)
		}
		if n > 0 {
			buffer.Write(tempBuffer[:n])
		}
		if err == io.EOF {
			break
		}
	}

	return buffer.Bytes(), nil
}
