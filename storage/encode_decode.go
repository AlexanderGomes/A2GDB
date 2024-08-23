package storage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
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

// CHECKED
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

func EncodePageHeader(header PageHeader, buf *bytes.Buffer) error {
	if err := binary.Write(buf, binary.LittleEndian, header.ID); err != nil {
		return err
	}
	if err := binary.Write(buf, binary.LittleEndian, header.LowerPtr); err != nil {
		return err
	}
	if err := binary.Write(buf, binary.LittleEndian, header.UpperPtr); err != nil {
		return err
	}
	if err := binary.Write(buf, binary.LittleEndian, header.NumTuples); err != nil {
		return err
	}
	return nil
}

func DecodePageHeader(header *PageHeader, buf *bytes.Buffer) error {
	if err := binary.Read(buf, binary.LittleEndian, &header.ID); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &header.LowerPtr); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &header.UpperPtr); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &header.NumTuples); err != nil {
		return err
	}
	return nil
}

func EncodePageV2(page *PageV2) ([]byte, error) {
	var buf bytes.Buffer

	if err := EncodePageHeader(page.Header, &buf); err != nil {
		return nil, err
	}

	if _, err := buf.Write(page.Data); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func DecodePageV2(data []byte) (*PageV2, error) {
	buf := bytes.NewBuffer(data)

	var header PageHeader
	if err := DecodePageHeader(&header, buf); err != nil {
		return nil, err
	}

	pageData := make([]byte, PageDataSize)
	if _, err := buf.Read(pageData); err != nil {
		return nil, err
	}

	page := &PageV2{
		Header:       header,
		Data:         pageData,
		PointerArray: []TupleLocation{},
	}

	return page, nil
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

func SerializeCatalog(catalog *Catalog) ([]byte, error) {
	var buf bytes.Buffer

	numTables := uint32(len(catalog.Tables))
	if err := binary.Write(&buf, binary.LittleEndian, numTables); err != nil {
		return nil, err
	}

	for tableName, tableInfo := range catalog.Tables {
		nameLen := uint32(len(tableName))
		if err := binary.Write(&buf, binary.LittleEndian, nameLen); err != nil {
			return nil, err
		}
		if _, err := buf.WriteString(string(tableName)); err != nil {
			return nil, err
		}

		schemaLen := uint32(len(tableInfo.Schema))
		if err := binary.Write(&buf, binary.LittleEndian, schemaLen); err != nil {
			return nil, err
		}
		for columnName, columnType := range tableInfo.Schema {
			colNameLen := uint32(len(columnName))
			if err := binary.Write(&buf, binary.LittleEndian, colNameLen); err != nil {
				return nil, err
			}
			if _, err := buf.WriteString(columnName); err != nil {
				return nil, err
			}

			if err := binary.Write(&buf, binary.LittleEndian, columnType.IsIndex); err != nil {
				return nil, err
			}
			colTypeLen := uint32(len(columnType.Type))
			if err := binary.Write(&buf, binary.LittleEndian, colTypeLen); err != nil {
				return nil, err
			}
			if _, err := buf.WriteString(columnType.Type); err != nil {
				return nil, err
			}
		}

		if err := binary.Write(&buf, binary.LittleEndian, tableInfo.NumOfPages); err != nil {
			return nil, err
		}

	}

	return buf.Bytes(), nil
}

func DeserializeCatalog(data []byte) (*Catalog, error) {
	var buf bytes.Buffer
	buf.Write(data)

	var catalog Catalog
	catalog.Tables = make(map[TableName]TableInfo)

	var numTables uint32
	if err := binary.Read(&buf, binary.LittleEndian, &numTables); err != nil {
		return nil, err
	}

	for i := uint32(0); i < numTables; i++ {
		var nameLen uint32
		if err := binary.Read(&buf, binary.LittleEndian, &nameLen); err != nil {
			return nil, err
		}
		tableName := make([]byte, nameLen)
		if _, err := buf.Read(tableName); err != nil {
			return nil, err
		}

		var tableInfo TableInfo

		var schemaLen uint32
		if err := binary.Read(&buf, binary.LittleEndian, &schemaLen); err != nil {
			return nil, err
		}
		tableInfo.Schema = make(map[string]ColumnType)
		for j := uint32(0); j < schemaLen; j++ {
			var colNameLen uint32
			if err := binary.Read(&buf, binary.LittleEndian, &colNameLen); err != nil {
				return nil, err
			}
			colName := make([]byte, colNameLen)
			if _, err := buf.Read(colName); err != nil {
				return nil, err
			}

			var isIndex bool
			if err := binary.Read(&buf, binary.LittleEndian, &isIndex); err != nil {
				return nil, err
			}
			var typeLen uint32
			if err := binary.Read(&buf, binary.LittleEndian, &typeLen); err != nil {
				return nil, err
			}
			colType := make([]byte, typeLen)
			if _, err := buf.Read(colType); err != nil {
				return nil, err
			}

			tableInfo.Schema[string(colName)] = ColumnType{
				IsIndex: isIndex,
				Type:    string(colType),
			}
		}

		if err := binary.Read(&buf, binary.LittleEndian, &tableInfo.NumOfPages); err != nil {
			return nil, err
		}

		catalog.Tables[TableName(tableName)] = tableInfo
	}

	return &catalog, nil
}

func EncodeItems(items []Item) ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.LittleEndian, int32(len(items))); err != nil {
		return nil, fmt.Errorf("EncodeItem: %w", err)
	}

	for _, item := range items {
		encodedItem, err := EncodeItem(item)
		if err != nil {
			return nil, fmt.Errorf("EncodeItem: %w", err)
		}
		buf.Write(encodedItem)
	}

	return buf.Bytes(), nil
}

func EncodeItem(item Item) ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.LittleEndian, item.Key); err != nil {
		return nil, err
	}

	if err := binary.Write(buf, binary.LittleEndian, item.Value); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func DecodeItems(data []byte) ([]Item, error) {
	buf := bytes.NewReader(data)
	var items []Item

	var length int32
	if err := binary.Read(buf, binary.LittleEndian, &length); err != nil {
		return nil, err
	}

	for i := int32(0); i < length; i++ {
		itemData := make([]byte, 16)
		if _, err := buf.Read(itemData); err != nil {
			return nil, err
		}
		item, err := DecodeItem(itemData)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, nil
}

func DecodeItem(data []byte) (Item, error) {
	buf := bytes.NewReader(data)
	var item Item

	if err := binary.Read(buf, binary.LittleEndian, &item.Key); err != nil {
		return Item{}, err
	}

	if err := binary.Read(buf, binary.LittleEndian, &item.Value); err != nil {
		return Item{}, err
	}

	return item, nil
}
