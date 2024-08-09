package storage

import "fmt"

const (
	PageSizeV2     = 4 * 1024
	HeaderSize     = 8
	TupleEntrySize = 4
)

type PageHeader struct {
	LowerPtr     uint16
	UpperPtr     uint16
	NumTuples    uint16
	CurrPageSize uint16
}

type PageV2 struct {
	Header       PageHeader
	PointerArray []TupleLocation
	Data         []byte
}

type TupleLocation struct {
	Offset uint16
	Length uint16
}

type TupleHeader struct {
	Length uint16
	Flags  uint8
}

type Tuple struct {
	Header TupleHeader
	Data   []byte
}

func CreatePageV2() *PageV2 {
	return &PageV2{
		Header: PageHeader{
			LowerPtr:     uint16(HeaderSize + 1),
			UpperPtr:     uint16(PageSizeV2 - HeaderSize),
			NumTuples:    0,
			CurrPageSize: HeaderSize,
		},
		PointerArray: []TupleLocation{},
		Data:         make([]byte, PageSizeV2-HeaderSize),
	}
}

func (p *PageV2) AddTuple(data []byte) error {
	tupleLen := uint16(len(data))
	offset := p.Header.UpperPtr - tupleLen
	canInsert := PageSizeV2-p.Header.CurrPageSize > tupleLen

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
	p.Header.CurrPageSize += (tupleLen + TupleEntrySize)

	p.Header.LowerPtr += TupleEntrySize
	return nil
}
