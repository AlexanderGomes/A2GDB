package storage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	NIL_FLAG        = int32(0)
	NODE_FLAG       = int32(1)
	RECORD_FLAG     = int32(2)
	PARENT_FLAG     = int32(3)
	NEXT_FLAG       = int32(4)
	OWN_PARENT_FLAG = int32(-1)
)

func EncodeDirectory(dir *DirectoryPageV2) ([]byte, error) {
	var buf bytes.Buffer

	numEntries := uint32(len(dir.Value))
	if err := binary.Write(&buf, binary.LittleEndian, numEntries); err != nil {
		return nil, err
	}

	for id, pageInfo := range dir.Value {
		if err := binary.Write(&buf, binary.LittleEndian, id); err != nil {
			return nil, err
		}

		encodedPageInfo, err := EncodePageInfo(pageInfo)
		if err != nil {
			return nil, err
		}

		length := uint32(len(encodedPageInfo))
		if err := binary.Write(&buf, binary.LittleEndian, length); err != nil {
			return nil, err
		}

		if _, err := buf.Write(encodedPageInfo); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func DecodeDirectory(data []byte) (*DirectoryPageV2, error) {
	buf := bytes.NewReader(data)

	var dir DirectoryPageV2

	var numEntries uint32
	if err := binary.Read(buf, binary.LittleEndian, &numEntries); err != nil {
		return nil, fmt.Errorf("error reading number of entries: %w", err)
	}

	dir.Value = make(map[PageID]*PageInfo, numEntries)

	for i := uint32(0); i < numEntries; i++ {
		var id PageID
		if err := binary.Read(buf, binary.LittleEndian, &id); err != nil {
			return nil, fmt.Errorf("error reading PageID: %w", err)
		}

		var length uint32
		if err := binary.Read(buf, binary.LittleEndian, &length); err != nil {
			return nil, fmt.Errorf("error reading length of PageInfo: %w", err)
		}

		encodedPageInfo := make([]byte, length)
		if _, err := io.ReadFull(buf, encodedPageInfo); err != nil {
			return nil, fmt.Errorf("error reading encoded PageInfo data: %w", err)
		}

		pageInfo, err := DecodePageInfo(encodedPageInfo)
		if err != nil {
			return nil, fmt.Errorf("error decoding PageInfo: %w", err)
		}

		dir.Value[id] = pageInfo
	}

	return &dir, nil
}

func DecodePageInfo(data []byte) (*PageInfo, error) {
	buf := bytes.NewReader(data)

	var pageInfo PageInfo

	if err := binary.Read(buf, binary.LittleEndian, &pageInfo.Offset); err != nil {
		return nil, fmt.Errorf("error reading Offset: %w", err)
	}

	var numTuples uint32
	if err := binary.Read(buf, binary.LittleEndian, &numTuples); err != nil {
		return nil, fmt.Errorf("error reading number of tuples: %w", err)
	}

	pageInfo.PointerArray = make([]TupleLocation, numTuples)
	for i := uint32(0); i < numTuples; i++ {
		var tuple TupleLocation
		if err := binary.Read(buf, binary.LittleEndian, &tuple.Offset); err != nil {
			return nil, fmt.Errorf("error reading TupleLocation.Offset: %w", err)
		}
		if err := binary.Read(buf, binary.LittleEndian, &tuple.Length); err != nil {
			return nil, fmt.Errorf("error reading TupleLocation.Length: %w", err)
		}
		pageInfo.PointerArray[i] = tuple
	}

	return &pageInfo, nil
}

func EncodePageInfo(pageInfo *PageInfo) ([]byte, error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.LittleEndian, pageInfo.Offset); err != nil {
		return nil, err
	}

	numTuples := uint32(len(pageInfo.PointerArray))
	if err := binary.Write(&buf, binary.LittleEndian, numTuples); err != nil {
		return nil, err
	}

	for _, tuple := range pageInfo.PointerArray {
		if err := binary.Write(&buf, binary.LittleEndian, tuple.Offset); err != nil {
			return nil, err
		}
		if err := binary.Write(&buf, binary.LittleEndian, tuple.Length); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func EncodePageV2(page *PageV2) ([]byte, error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.LittleEndian, page.Header); err != nil {
		return nil, err
	}

	if _, err := buf.Write(page.Data); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func DecodePageV2(data []byte) (*PageV2, error) {
	buf := bytes.NewReader(data)
	var page PageV2

	if err := binary.Read(buf, binary.LittleEndian, &page.Header); err != nil {
		return nil, err
	}

	page.Data = make([]byte, PageDataSize)
	if _, err := buf.Read(page.Data); err != nil {
		return nil, err
	}

	return &page, nil
}

func SerializeRow(row *RowV2) ([]byte, error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.LittleEndian, row.ID); err != nil {
		return nil, err
	}

	numValues := uint32(len(row.Values))
	if err := binary.Write(&buf, binary.LittleEndian, numValues); err != nil {
		return nil, err
	}

	for key, value := range row.Values {
		if err := binary.Write(&buf, binary.LittleEndian, uint32(len(key))); err != nil {
			return nil, err
		}
		if _, err := buf.WriteString(key); err != nil {
			return nil, err
		}

		if err := binary.Write(&buf, binary.LittleEndian, uint32(len(value))); err != nil {
			return nil, err
		}
		if _, err := buf.WriteString(value); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func DecodeRow(data []byte) (*RowV2, error) {
	buf := bytes.NewReader(data)

	var row RowV2

	if err := binary.Read(buf, binary.LittleEndian, &row.ID); err != nil {
		return nil, fmt.Errorf("error reading Row ID: %w", err)
	}

	var numValues uint32
	if err := binary.Read(buf, binary.LittleEndian, &numValues); err != nil {
		return nil, fmt.Errorf("error reading number of values: %w", err)
	}

	row.Values = make(map[string]string, numValues)

	for i := uint32(0); i < numValues; i++ {
		var keyLength uint32
		if err := binary.Read(buf, binary.LittleEndian, &keyLength); err != nil {
			return nil, fmt.Errorf("error reading key length: %w", err)
		}

		keyBytes := make([]byte, keyLength)
		if _, err := io.ReadFull(buf, keyBytes); err != nil {
			return nil, fmt.Errorf("error reading key: %w", err)
		}
		key := string(keyBytes)

		var valueLength uint32
		if err := binary.Read(buf, binary.LittleEndian, &valueLength); err != nil {
			return nil, fmt.Errorf("error reading value length: %w", err)
		}

		valueBytes := make([]byte, valueLength)
		if _, err := io.ReadFull(buf, valueBytes); err != nil {
			return nil, fmt.Errorf("error reading value: %w", err)
		}
		value := string(valueBytes)

		row.Values[key] = value
	}

	return &row, nil
}

func ResetBytesToEmpty(page *PageV2, offset uint16, length uint16) error {
	if offset+length > uint16(len(page.Data)) {
		return fmt.Errorf("offset and length exceed page data bounds")
	}

	for i := uint16(0); i < length; i++ {
		page.Data[offset+i] = 0x0
	}

	return nil
}

// func EncodeBp(leafMap map[uint64]*Record) ([]byte, error) {
// 	var buf bytes.Buffer

// 	for key, record := range leafMap {

// 		if err := binary.Write(&buf, binary.BigEndian, key); err != nil {
// 			return nil, err
// 		}

// 		valueLength := uint32(len(record.Value))
// 		if err := binary.Write(&buf, binary.BigEndian, valueLength); err != nil {
// 			return nil, err
// 		}

// 		if _, err := buf.Write(record.Value); err != nil {
// 			return nil, err
// 		}
// 	}

// 	return buf.Bytes(), nil
// }


// func DecodeBp(data []byte) (*Node, error) {
// 	var node Node

// 	return &node, nil
// }
