package engines

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"runtime/metrics"
	"time"

	"github.com/axiomhq/hyperloglog"
	"github.com/bits-and-blooms/bloom/v3"
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
	buf := bytes.NewReader(data)
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

func readBytes(buf *bytes.Reader) ([]byte, error) {
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

// Helper functions for serialization
func writeString(buf *bytes.Buffer, s string) error {
	nameLen := uint32(len(s))
	if err := binary.Write(buf, binary.LittleEndian, nameLen); err != nil {
		return err
	}
	if _, err := buf.WriteString(s); err != nil {
		return err
	}
	return nil
}

func readString(buf *bytes.Reader) (string, error) {
	var strLen uint32
	if err := binary.Read(buf, binary.LittleEndian, &strLen); err != nil {
		return "", err
	}

	strBytes := make([]byte, strLen)
	if _, err := buf.Read(strBytes); err != nil {
		return "", err
	}
	return string(strBytes), nil
}

func writeColumnType(buf *bytes.Buffer, columnType ColumnType) error {
	if err := binary.Write(buf, binary.LittleEndian, columnType.IsIndex); err != nil {
		return err
	}
	return writeString(buf, columnType.Type)
}

func readColumnType(buf *bytes.Reader) (ColumnType, error) {
	var isIndex bool
	if err := binary.Read(buf, binary.LittleEndian, &isIndex); err != nil {
		return ColumnType{}, err
	}

	colType, err := readString(buf)
	if err != nil {
		return ColumnType{}, err
	}

	return ColumnType{
		IsIndex: isIndex,
		Type:    colType,
	}, nil
}

func writeSchema(buf *bytes.Buffer, schema map[string]ColumnType) error {
	schemaLen := uint32(len(schema))
	if err := binary.Write(buf, binary.LittleEndian, schemaLen); err != nil {
		return err
	}

	for columnName, columnType := range schema {
		if err := writeString(buf, columnName); err != nil {
			return err
		}
		if err := writeColumnType(buf, columnType); err != nil {
			return err
		}
	}
	return nil
}

func readSchema(buf *bytes.Reader) (map[string]ColumnType, error) {
	var schemaLen uint32
	if err := binary.Read(buf, binary.LittleEndian, &schemaLen); err != nil {
		return nil, err
	}

	schema := make(map[string]ColumnType)
	for j := uint32(0); j < schemaLen; j++ {
		colName, err := readString(buf)
		if err != nil {
			return nil, err
		}

		columnType, err := readColumnType(buf)
		if err != nil {
			return nil, err
		}

		schema[colName] = columnType
	}
	return schema, nil
}

func writeColumnAvgWidth(buf *bytes.Buffer, columnAvgWidth map[Column]uint16) error {
	mapLen := uint32(len(columnAvgWidth))
	if err := binary.Write(buf, binary.LittleEndian, mapLen); err != nil {
		return err
	}

	for col, width := range columnAvgWidth {
		if err := writeString(buf, string(col)); err != nil {
			return err
		}
		if err := binary.Write(buf, binary.LittleEndian, width); err != nil {
			return err
		}
	}
	return nil
}

func readColumnAvgWidth(buf *bytes.Reader) (map[Column]uint16, error) {
	var mapLen uint32
	if err := binary.Read(buf, binary.LittleEndian, &mapLen); err != nil {
		return nil, err
	}

	columnAvgWidth := make(map[Column]uint16)
	for i := uint32(0); i < mapLen; i++ {
		colName, err := readString(buf)
		if err != nil {
			return nil, err
		}

		var width uint16
		if err := binary.Read(buf, binary.LittleEndian, &width); err != nil {
			return nil, err
		}

		columnAvgWidth[Column(colName)] = width
	}
	return columnAvgWidth, nil
}

func writeHistogram(buf *bytes.Buffer, histogram map[Column]*metrics.Float64Histogram) error {
	mapLen := uint32(len(histogram))
	if err := binary.Write(buf, binary.LittleEndian, mapLen); err != nil {
		return err
	}

	for col, hist := range histogram {
		if err := writeString(buf, string(col)); err != nil {
			return err
		}

		countsLen := uint32(len(hist.Counts))
		if err := binary.Write(buf, binary.LittleEndian, countsLen); err != nil {
			return err
		}
		for _, count := range hist.Counts {
			if err := binary.Write(buf, binary.LittleEndian, count); err != nil {
				return err
			}
		}

		bucketsLen := uint32(len(hist.Buckets))
		if err := binary.Write(buf, binary.LittleEndian, bucketsLen); err != nil {
			return err
		}
		for _, bucket := range hist.Buckets {
			if err := binary.Write(buf, binary.LittleEndian, bucket); err != nil {
				return err
			}
		}
	}
	return nil
}

func readHistogram(buf *bytes.Reader) (map[Column]*metrics.Float64Histogram, error) {
	var mapLen uint32
	if err := binary.Read(buf, binary.LittleEndian, &mapLen); err != nil {
		return nil, err
	}

	histogram := make(map[Column]*metrics.Float64Histogram)
	for i := uint32(0); i < mapLen; i++ {
		colName, err := readString(buf)
		if err != nil {
			return nil, err
		}

		hist := &metrics.Float64Histogram{}

		var countsLen uint32
		if err := binary.Read(buf, binary.LittleEndian, &countsLen); err != nil {
			return nil, err
		}
		hist.Counts = make([]uint64, countsLen)
		for j := uint32(0); j < countsLen; j++ {
			if err := binary.Read(buf, binary.LittleEndian, &hist.Counts[j]); err != nil {
				return nil, err
			}
		}

		var bucketsLen uint32
		if err := binary.Read(buf, binary.LittleEndian, &bucketsLen); err != nil {
			return nil, err
		}
		hist.Buckets = make([]float64, bucketsLen)
		for j := uint32(0); j < bucketsLen; j++ {
			if err := binary.Read(buf, binary.LittleEndian, &hist.Buckets[j]); err != nil {
				return nil, err
			}
		}

		histogram[Column(colName)] = hist
	}
	return histogram, nil
}

func writeUniqueCount(buf *bytes.Buffer, sketch *hyperloglog.Sketch) error {
	if sketch == nil {
		if err := binary.Write(buf, binary.LittleEndian, false); err != nil {
			return err
		}
		return nil
	}

	if err := binary.Write(buf, binary.LittleEndian, true); err != nil {
		return err
	}

	sketchData, err := sketch.MarshalBinary()
	if err != nil {
		return err
	}

	dataLen := uint32(len(sketchData))
	if err := binary.Write(buf, binary.LittleEndian, dataLen); err != nil {
		return err
	}

	if _, err := buf.Write(sketchData); err != nil {
		return err
	}

	return nil
}

func readUniqueCount(buf *bytes.Reader) (*hyperloglog.Sketch, error) {
	var hasSketch bool
	if err := binary.Read(buf, binary.LittleEndian, &hasSketch); err != nil {
		return nil, err
	}

	if !hasSketch {
		return nil, nil
	}

	var dataLen uint32
	if err := binary.Read(buf, binary.LittleEndian, &dataLen); err != nil {
		return nil, err
	}

	sketchData := make([]byte, dataLen)
	if _, err := buf.Read(sketchData); err != nil {
		return nil, err
	}

	sketch := &hyperloglog.Sketch{}
	if err := sketch.UnmarshalBinary(sketchData); err != nil {
		return nil, err
	}

	return sketch, nil
}

func writeSkipPage(buf *bytes.Buffer, skipPage map[PageID]map[Column]*bloom.BloomFilter) error {
	mapLen := uint32(len(skipPage))
	if err := binary.Write(buf, binary.LittleEndian, mapLen); err != nil {
		return err
	}

	for pageID, colMap := range skipPage {
		if err := binary.Write(buf, binary.LittleEndian, pageID); err != nil {
			return err
		}

		colMapLen := uint32(len(colMap))
		if err := binary.Write(buf, binary.LittleEndian, colMapLen); err != nil {
			return err
		}

		for col, filter := range colMap {
			if err := writeString(buf, string(col)); err != nil {
				return err
			}

			filterData, err := filter.MarshalBinary()
			if err != nil {
				return err
			}

			dataLen := uint32(len(filterData))
			if err := binary.Write(buf, binary.LittleEndian, dataLen); err != nil {
				return err
			}

			if _, err := buf.Write(filterData); err != nil {
				return err
			}
		}
	}
	return nil
}

func readSkipPage(buf *bytes.Reader) (map[PageID]map[Column]*bloom.BloomFilter, error) {
	var mapLen uint32
	if err := binary.Read(buf, binary.LittleEndian, &mapLen); err != nil {
		return nil, err
	}

	skipPage := make(map[PageID]map[Column]*bloom.BloomFilter)
	for i := uint32(0); i < mapLen; i++ {
		var pageID PageID
		if err := binary.Read(buf, binary.LittleEndian, &pageID); err != nil {
			return nil, err
		}

		var colMapLen uint32
		if err := binary.Read(buf, binary.LittleEndian, &colMapLen); err != nil {
			return nil, err
		}

		colMap := make(map[Column]*bloom.BloomFilter)
		for j := uint32(0); j < colMapLen; j++ {
			colName, err := readString(buf)
			if err != nil {
				return nil, err
			}

			var dataLen uint32
			if err := binary.Read(buf, binary.LittleEndian, &dataLen); err != nil {
				return nil, err
			}

			filterData := make([]byte, dataLen)
			if _, err := buf.Read(filterData); err != nil {
				return nil, err
			}

			filter := &bloom.BloomFilter{}
			if err := filter.UnmarshalBinary(filterData); err != nil {
				return nil, err
			}

			colMap[Column(colName)] = filter
		}

		skipPage[pageID] = colMap
	}
	return skipPage, nil
}

func writeTableInfo(buf *bytes.Buffer, tableInfo *TableInfo) error {
	if err := writeSchema(buf, tableInfo.Schema); err != nil {
		return err
	}

	if err := binary.Write(buf, binary.LittleEndian, tableInfo.NumOfPages); err != nil {
		return err
	}
	if err := binary.Write(buf, binary.LittleEndian, tableInfo.UsedSpace); err != nil {
		return err
	}

	return nil
}

func readTableInfo(buf *bytes.Reader) (*TableInfo, error) {
	var tableInfo TableInfo

	schema, err := readSchema(buf)
	if err != nil {
		return nil, err
	}
	tableInfo.Schema = schema

	if err := binary.Read(buf, binary.LittleEndian, &tableInfo.NumOfPages); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &tableInfo.UsedSpace); err != nil {
		return nil, err
	}

	return &tableInfo, nil
}

func SerializeCatalog(catalog *Catalog) ([]byte, error) {
	var buf bytes.Buffer

	numTables := uint32(len(catalog.Tables))
	if err := binary.Write(&buf, binary.LittleEndian, numTables); err != nil {
		return nil, err
	}

	for tableName, tableInfo := range catalog.Tables {
		if err := writeString(&buf, tableName); err != nil {
			return nil, err
		}

		if err := writeTableInfo(&buf, tableInfo); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func DeserializeCatalog(data []byte) (*Catalog, error) {
	buf := bytes.NewReader(data)

	var catalog Catalog
	catalog.Tables = make(map[string]*TableInfo)

	var numTables uint32
	if err := binary.Read(buf, binary.LittleEndian, &numTables); err != nil {
		return nil, err
	}

	for i := uint32(0); i < numTables; i++ {
		tableName, err := readString(buf)
		if err != nil {
			return nil, err
		}

		tableInfo, err := readTableInfo(buf)
		if err != nil {
			return nil, err
		}

		catalog.Tables[tableName] = tableInfo
	}

	return &catalog, nil
}
