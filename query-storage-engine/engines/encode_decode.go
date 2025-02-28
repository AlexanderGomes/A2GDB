package engines

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"
)

func EncodeMemObj(memObj map[uint16][]*FreeSpace) ([]byte, error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.LittleEndian, int16(len(memObj))); err != nil {
		return nil, err
	}

	for key, freeSpaces := range memObj {
		if err := binary.Write(&buf, binary.LittleEndian, key); err != nil {
			return nil, err
		}

		if err := binary.Write(&buf, binary.LittleEndian, int16(len(freeSpaces))); err != nil {
			return nil, err
		}

		for _, fs := range freeSpaces {
			if err := binary.Write(&buf, binary.LittleEndian, fs.PageID); err != nil {
				return nil, err
			}

			if err := binary.Write(&buf, binary.LittleEndian, fs.FreeMemory); err != nil {
				return nil, err
			}
		}
	}

	return buf.Bytes(), nil
}

func DecodeMemObj(data []byte) (map[uint16][]*FreeSpace, error) {
	buf := bytes.NewReader(data)
	memObj := make(map[uint16][]*FreeSpace)

	var numKeys int16
	if err := binary.Read(buf, binary.LittleEndian, &numKeys); err != nil {
		return nil, err
	}

	for i := int16(0); i < numKeys; i++ {
		var key uint16
		if err := binary.Read(buf, binary.LittleEndian, &key); err != nil {
			return nil, err
		}

		var numFreeSpaces int16
		if err := binary.Read(buf, binary.LittleEndian, &numFreeSpaces); err != nil {
			return nil, err
		}

		freeSpaces := make([]*FreeSpace, numFreeSpaces)
		for j := int16(0); j < numFreeSpaces; j++ {
			fs := &FreeSpace{}

			if err := binary.Read(buf, binary.LittleEndian, &fs.PageID); err != nil {
				return nil, err
			}

			if err := binary.Read(buf, binary.LittleEndian, &fs.FreeMemory); err != nil {
				return nil, err
			}

			freeSpaces[j] = fs
		}

		memObj[key] = freeSpaces
	}

	return memObj, nil
}

func EncodeDirectory(dir *DirectoryPageV2) ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, 1024*20))

	numEntries := uint32(len(dir.Value))
	if err := binary.Write(buf, binary.LittleEndian, numEntries); err != nil {
		return nil, err
	}

	for id, pageInfo := range dir.Value {
		if err := binary.Write(buf, binary.LittleEndian, id); err != nil {
			return nil, err
		}

		encodedPageInfo, err := EncodePageInfo(pageInfo)
		if err != nil {
			return nil, err
		}

		length := uint32(len(encodedPageInfo))
		if err := binary.Write(buf, binary.LittleEndian, length); err != nil {
			return nil, err
		}

		if _, err := buf.Write(encodedPageInfo); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func DecodePageInfo(data []byte) (*PageInfo, error) {
	buf := bytes.NewReader(data)

	var pageInfo PageInfo

	if err := binary.Read(buf, binary.LittleEndian, &pageInfo.Offset); err != nil {
		return nil, err
	}

	if err := binary.Read(buf, binary.LittleEndian, &pageInfo.Level); err != nil {
		return nil, err
	}

	if err := binary.Read(buf, binary.LittleEndian, &pageInfo.ExactFreeMem); err != nil {
		return nil, err
	}

	var numTuples uint32
	if err := binary.Read(buf, binary.LittleEndian, &numTuples); err != nil {
		return nil, err
	}

	pageInfo.PointerArray = make([]TupleLocation, numTuples)
	for i := uint32(0); i < numTuples; i++ {
		var tuple TupleLocation
		if err := binary.Read(buf, binary.LittleEndian, &tuple.Offset); err != nil {
			return nil, err
		}

		if err := binary.Read(buf, binary.LittleEndian, &tuple.Length); err != nil {
			return nil, err
		}

		freeByte, err := buf.ReadByte()
		if err != nil {
			return nil, err
		}

		tuple.Free = freeByte == 1

		pageInfo.PointerArray[i] = tuple
	}

	return &pageInfo, nil
}

func EncodePageInfo(pageInfo *PageInfo) ([]byte, error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.LittleEndian, pageInfo.Offset); err != nil {
		return nil, err
	}

	if err := binary.Write(&buf, binary.LittleEndian, pageInfo.Level); err != nil {
		return nil, err
	}

	if err := binary.Write(&buf, binary.LittleEndian, pageInfo.ExactFreeMem); err != nil {
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

		freeByte := byte(0)
		if tuple.Free {
			freeByte = 1
		}

		if err := buf.WriteByte(freeByte); err != nil {
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
		return nil, err
	}

	dir.Value = make(map[PageID]*PageInfo, numEntries)

	for i := uint32(0); i < numEntries; i++ {
		var id PageID
		if err := binary.Read(buf, binary.LittleEndian, &id); err != nil {
			return nil, err
		}

		var length uint32
		if err := binary.Read(buf, binary.LittleEndian, &length); err != nil {
			return nil, err
		}

		encodedPageInfo := make([]byte, length)
		if _, err := io.ReadFull(buf, encodedPageInfo); err != nil {
			return nil, err
		}

		pageInfo, err := DecodePageInfo(encodedPageInfo)
		if err != nil {
			return nil, err
		}

		dir.Value[id] = pageInfo
	}

	return &dir, nil
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

func EncodeRow(row *RowV2) ([]byte, error) {
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
		return nil, err
	}
	var numValues uint32
	if err := binary.Read(buf, binary.LittleEndian, &numValues); err != nil {
		return nil, err
	}

	row.Values = make(map[string]string, numValues)

	for i := uint32(0); i < numValues; i++ {
		var keyLength uint32
		if err := binary.Read(buf, binary.LittleEndian, &keyLength); err != nil {
			return nil, err
		}

		keyBytes := make([]byte, keyLength)
		if _, err := io.ReadFull(buf, keyBytes); err != nil {
			return nil, err
		}
		key := string(keyBytes)

		var valueLength uint32
		if err := binary.Read(buf, binary.LittleEndian, &valueLength); err != nil {
			return nil, err
		}

		valueBytes := make([]byte, valueLength)
		if _, err := io.ReadFull(buf, valueBytes); err != nil {
			return nil, err
		}
		value := string(valueBytes)

		row.Values[key] = value
	}

	return &row, nil
}

func ResetBytesToEmpty(page *PageV2, offset uint16, length uint16) error {
	if offset+length > uint16(len(page.Data)) {
		return errors.New("offset and length exceed page data bounds")
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
	catalog.Tables = make(map[string]*TableInfo)

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

		catalog.Tables[string(tableName)] = &tableInfo
	}

	return &catalog, nil
}

func EncodeItems(items []Item) ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.LittleEndian, int32(len(items))); err != nil {
		return nil, err
	}

	for _, item := range items {
		encodedItem, err := EncodeItem(item)
		if err != nil {
			return nil, fmt.Errorf("EncodeItem failed: %w", err)
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
			return nil, fmt.Errorf("DecodeItem failed: %w", err)
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

func encodeLog(log *LogRecord) ([]byte, error) {
	data, err := serializeLogRecord(log)
	if err != nil {
		return nil, err
	}

	length := uint32(len(data))
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.BigEndian, length); err != nil {
		return nil, err
	}

	if _, err := buf.Write(data); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func serializeLogRecord(log *LogRecord) ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.BigEndian, log.LSN); err != nil {
		return nil, err
	}

	if err := binary.Write(buf, binary.BigEndian, log.Type); err != nil {
		return nil, err
	}

	if err := writeString(buf, log.TxID); err != nil {
		return nil, err
	}

	if err := writeString(buf, log.TableID); err != nil {
		return nil, err
	}

	if err := binary.Write(buf, binary.BigEndian, log.RowID); err != nil {
		return nil, err
	}

	if err := writeBytes(buf, log.BeforeImage); err != nil {
		return nil, err
	}

	if err := writeBytes(buf, log.AfterImage); err != nil {
		return nil, err
	}

	if err := binary.Write(buf, binary.BigEndian, log.Timestamp.UnixNano()); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func writeBytes(buf *bytes.Buffer, b []byte) error {
	length := uint32(len(b))
	if err := binary.Write(buf, binary.BigEndian, length); err != nil {
		return err
	}

	if _, err := buf.Write(b); err != nil {
		return err
	}

	return nil
}

func writeString(buf *bytes.Buffer, s string) error {
	length := uint16(len(s))
	if err := binary.Write(buf, binary.BigEndian, length); err != nil {
		return err
	}

	if _, err := buf.WriteString(s); err != nil {
		return err
	}

	return nil
}

func decodeLength(encodedData []byte) (uint32, error) {
	if len(encodedData) < 4 {
		return 0, errors.New("encoded data too short to contain length")
	}

	buf := bytes.NewReader(encodedData)

	var length uint32
	if err := binary.Read(buf, binary.BigEndian, &length); err != nil {
		return 0, err
	}

	return length, nil
}

func deserializeLogRecord(data []byte) (*LogRecord, error) {
	buf := bytes.NewBuffer(data)
	log := &LogRecord{}

	if err := binary.Read(buf, binary.BigEndian, &log.LSN); err != nil {
		return nil, err
	}

	if err := binary.Read(buf, binary.BigEndian, &log.Type); err != nil {
		return nil, err
	}

	txID, err := readString(buf)
	if err != nil {
		return nil, err
	}
	log.TxID = txID

	tableID, err := readString(buf)
	if err != nil {
		return nil, err
	}
	log.TableID = tableID

	if err := binary.Read(buf, binary.BigEndian, &log.RowID); err != nil {
		return nil, err
	}

	beforeImage, err := readBytes(buf)
	if err != nil {
		return nil, err
	}
	log.BeforeImage = beforeImage

	afterImage, err := readBytes(buf)
	if err != nil {
		return nil, err
	}
	log.AfterImage = afterImage

	var timestampNano int64
	if err := binary.Read(buf, binary.BigEndian, &timestampNano); err != nil {
		return nil, err
	}
	log.Timestamp = time.Unix(0, timestampNano)

	return log, nil
}

func readString(buf *bytes.Buffer) (string, error) {
	var length uint16
	if err := binary.Read(buf, binary.BigEndian, &length); err != nil {
		return "", err
	}

	strBytes := make([]byte, length)
	if _, err := buf.Read(strBytes); err != nil {
		return "", err
	}

	return string(strBytes), nil
}

func readBytes(buf *bytes.Buffer) ([]byte, error) {
	var length uint32
	if err := binary.Read(buf, binary.BigEndian, &length); err != nil {
		return nil, err
	}

	bytes := make([]byte, length)
	if _, err := buf.Read(bytes); err != nil {
		return nil, err
	}
	return bytes, nil
}

func DecodeReq(data []byte) (uint8, []byte, error) {
	var operation uint8

	buf := bytes.NewReader(data)

	if err := binary.Read(buf, binary.LittleEndian, &operation); err != nil {
		return 0, nil, err
	}

	remainingData := make([]byte, buf.Len())
	if _, err := buf.Read(remainingData); err != nil {
		return 0, nil, err
	}

	return operation, remainingData, nil
}
