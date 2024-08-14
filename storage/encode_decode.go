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

func EncodeNode(node *Node, visited map[*Node]bool) ([]byte, error) {
	var buf bytes.Buffer

	if visited[node] {
		return []byte{}, nil
	}

	visited[node] = true

	binary.Write(&buf, binary.LittleEndian, int32(len(node.Keys)))

	for _, key := range node.Keys {
		binary.Write(&buf, binary.LittleEndian, key)
	}

	binary.Write(&buf, binary.LittleEndian, int32(len(node.Pointers)))

	for _, p := range node.Pointers {
		if p == nil {
			binary.Write(&buf, binary.LittleEndian, NIL_FLAG)
			continue
		}

		switch p := p.(type) {
		case *Node:
			binary.Write(&buf, binary.LittleEndian, NODE_FLAG)
			encodedNode, err := EncodeNode(p, visited)
			if err != nil {
				return nil, err
			}
			binary.Write(&buf, binary.LittleEndian, int32(len(encodedNode)))
			buf.Write(encodedNode)
		case *Record:
			binary.Write(&buf, binary.LittleEndian, RECORD_FLAG)
			binary.Write(&buf, binary.LittleEndian, int32(len(p.Value)))
			buf.Write(p.Value)
		default:
			return nil, fmt.Errorf("unknown pointer type %T", p)
		}
	}

	if err := binary.Write(&buf, binary.LittleEndian, byte(0)); err != nil {
		return nil, err
	}

	if node.IsLeaf {
		buf.Bytes()[buf.Len()-1] = 1
	}

	binary.Write(&buf, binary.LittleEndian, int32(node.NumKeys))

	if node.Parent != nil {
		binary.Write(&buf, binary.LittleEndian, PARENT_FLAG)
		encodedNode, err := EncodeNode(node.Parent, visited)
		if err != nil {
			return nil, err
		}

		binary.Write(&buf, binary.LittleEndian, int32(len(encodedNode)))
		buf.Write(encodedNode)

	} else {
		binary.Write(&buf, binary.LittleEndian, NIL_FLAG)
	}

	if node.Next != nil {
		binary.Write(&buf, binary.LittleEndian, NEXT_FLAG)
		encodedNode, err := EncodeNode(node.Next, visited)
		if err != nil {
			return nil, err
		}
		binary.Write(&buf, binary.LittleEndian, int32(len(encodedNode)))
		buf.Write(encodedNode)
	} else {
		binary.Write(&buf, binary.LittleEndian, NIL_FLAG)
	}

	return buf.Bytes(), nil
}
func DecodeNode(data []byte) (*Node, error) {
	var visited = make(map[*Node]bool)

	return decodeNode(data, visited)
}

func decodeNode(data []byte, visited map[*Node]bool) (*Node, error) {
	var node Node
	buf := bytes.NewReader(data)
	

	var numKeys int32
	if err := binary.Read(buf, binary.LittleEndian, &numKeys); err != nil {
		return nil, fmt.Errorf("num keys: %w", err)
	}

	node.Keys = make([]uint64, numKeys)
	for i := int32(0); i < numKeys; i++ {
		if err := binary.Read(buf, binary.LittleEndian, &node.Keys[i]); err != nil {
			return nil, fmt.Errorf("reading keys: %w", err)
		}
	}

	var numPointers int32
	if err := binary.Read(buf, binary.LittleEndian, &numPointers); err != nil {
		return nil, fmt.Errorf("num pointers: %w", err)
	}

	node.Pointers = make([]interface{}, numPointers)
	for i := int32(0); i < numPointers; i++ {
		var flag int32
		if err := binary.Read(buf, binary.LittleEndian, &flag); err != nil {
			return nil, fmt.Errorf("reading pointers: %w", err)
		}

		switch flag {
		case NIL_FLAG:
			node.Pointers[i] = nil
		case NODE_FLAG:
			var encodedLength int32
			if err := binary.Read(buf, binary.LittleEndian, &encodedLength); err != nil {
				return nil, fmt.Errorf("node length: %w", err)
			}
			encodedNode := make([]byte, encodedLength)
			if _, err := buf.Read(encodedNode); err != nil {
				return nil, fmt.Errorf("reading node: %w", err)
			}

			childNode, err := decodeNode(encodedNode, visited)
			if err != nil {
				return nil, fmt.Errorf(err.Error())
			}
			node.Pointers[i] = childNode
		case RECORD_FLAG:
			var valueLen int32
			if err := binary.Read(buf, binary.LittleEndian, &valueLen); err != nil {
				return nil, fmt.Errorf("record length: %w", err)
			}
			value := make([]byte, valueLen)
			if _, err := buf.Read(value); err != nil {
				return nil, fmt.Errorf("reading record: %w", err)
			}
			node.Pointers[i] = &Record{Value: value}
		default:
			return nil, fmt.Errorf("unknown pointer flag %d", flag)
		}
	}

	var isLeafByte byte
	if err := binary.Read(buf, binary.LittleEndian, &isLeafByte); err != nil {
		return nil, fmt.Errorf("reading isLeaf flag: %w", err)
	}

	node.IsLeaf = isLeafByte != 0

	var numKey int32
	if err := binary.Read(buf, binary.LittleEndian, &numKey); err != nil {
		return nil, fmt.Errorf("total number of keys: %w", err)
	}

	node.NumKeys = int(numKey)

	// Read parent
	var parentFlag int32
	if err := binary.Read(buf, binary.LittleEndian, &parentFlag); err != nil {
		return nil, fmt.Errorf("parent flag: %w", err)
	}

	switch parentFlag {
	case NIL_FLAG:
		node.Parent = nil
	case PARENT_FLAG:
		var encodedLength int32
		if err := binary.Read(buf, binary.LittleEndian, &encodedLength); err != nil {
			return nil, fmt.Errorf("parent length: %w", err)
		}

		encodedNode := make([]byte, encodedLength)
		if _, err := buf.Read(encodedNode); err != nil {
			return nil, fmt.Errorf("encoded parent: %w", err)
		}

		if encodedLength == 0 {
			node.Parent = &node
		}

		parentNode, err := decodeNode(encodedNode, visited)
		if err != nil {
			return nil, fmt.Errorf("decoded parent: %w", err)
		}

		node.Parent = parentNode
	default:
		return nil, fmt.Errorf("unknown parent flag %d", parentFlag)
	}

	var nextFlag int32
	if err := binary.Read(buf, binary.LittleEndian, &nextFlag); err != nil {
		return nil, err
	}

	switch nextFlag {
	case NIL_FLAG:
		node.Next = nil
	case NEXT_FLAG:
		var encodedLength int32
		if err := binary.Read(buf, binary.LittleEndian, &encodedLength); err != nil {
			return nil, err
		}
		encodedNode := make([]byte, encodedLength)
		if _, err := buf.Read(encodedNode); err != nil {
			return nil, err
		}

		nextNode, err := decodeNode(encodedNode, visited)
		if err != nil {
			return nil, err
		}
		node.Next = nextNode
	default:
		return nil, fmt.Errorf("unknown next flag %d", nextFlag)
	}

	visited[&node] = true
	return &node, nil
}
