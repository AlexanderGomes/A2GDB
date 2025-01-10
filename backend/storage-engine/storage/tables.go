package storage

import (
	"a2gdb/logger"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/btree"
)

const (
	PERCENTAGE     = 20
	BUFFER_SIZE    = 2000
	NUM_DECODERS   = 20
	PAGES_PER_READ = 100
	MAX_FILE_SIZE  = 1 * 1024 * 1024 * 1024
)

type TableObj struct {
	DirectoryPage *DirectoryPageV2
	BpTree        *btree.BTree
	Memory        map[uint16][]*FreeSpace
	DirFile       *os.File
	BpFile        *os.File
	DataFile      *os.File
	MemFile       *os.File
	TableName     string
}

type FreeSpace struct {
	PageID      PageID
	FreeMemory  uint16
	TempPagePtr *PageV2
}

func (dm *DiskManagerV2) InMemoryTableSetUp(tableName string) (*TableObj, error) {
	dirObj, dirFilePtr, err := GetNonpageFile(dm.DBdirectory, tableName, "directory_page")
	if err != nil {
		return nil, fmt.Errorf("GetNonpageFile failed: %w", err)
	}

	bptreeObj, bptreeFilePtr, err := GetNonpageFile(dm.DBdirectory, tableName, "bptree")
	if err != nil {
		return nil, fmt.Errorf("GetNonpageFile failed: %w", err)
	}

	memObj, memFilePtr, err := GetNonpageFile(dm.DBdirectory, tableName, "freeMem")
	if err != nil {
		return nil, fmt.Errorf("GetNonpageFile failed: %w", err)
	}

	_, dataFilePtr, err := GetNonpageFile(dm.DBdirectory, tableName, tableName)
	if err != nil {
		return nil, fmt.Errorf("GetNonpageFile failed: %w", err)
	}

	tableObj := &TableObj{
		DirectoryPage: dirObj.(*DirectoryPageV2),
		BpTree:        bptreeObj.(*btree.BTree),
		Memory:        memObj.(map[uint16][]*FreeSpace),
		DataFile:      dataFilePtr,
		DirFile:       dirFilePtr,
		BpFile:        bptreeFilePtr,
		MemFile:       memFilePtr,
		TableName:     tableName,
	}

	dm.TableObjs[tableName] = tableObj
	return tableObj, nil
}

func GetNonpageFile(dbDirectory, tableName, fileName string) (interface{}, *os.File, error) {
	var object interface{}

	filePath := filepath.Join(dbDirectory, "Tables", tableName, fileName)
	filePtr, err := os.OpenFile(filePath, os.O_RDWR, 0666)
	if err != nil {
		return nil, nil, fmt.Errorf("opening non page file failed: %s", err)
	}

	switch fileName {
	case "bptree":
		object, err = GetBpTree(filePtr)
		if err != nil {
			return nil, nil, fmt.Errorf("getting bp data failed: %s", err)
		}
	case "directory_page":
		object, err = GetDirInfo(filePtr)
		if err != nil {
			return nil, nil, fmt.Errorf("getting directory data failed: %s", err)
		}
	case tableName:
		return nil, filePtr, nil
	case "freeMem":
		object, err = GetMemInfo(filePtr)
		if err != nil {
			return nil, nil, fmt.Errorf("getting mem obj failed: %s", err)
		}
	}

	return object, filePtr, nil
}

func GetMemInfo(fileptr *os.File) (map[uint16][]*FreeSpace, error) {
	byts, err := ReadNonPageFile(fileptr)
	if err != nil {
		return nil, fmt.Errorf("ReadNonPageFile failed: %w", err)
	}

	if len(byts) == 0 {
		return make(map[uint16][]*FreeSpace), nil
	}

	memObj, err := DecodeMemObj(byts)
	if err != nil {
		return nil, fmt.Errorf("DecodeMemObj failed: %w", err)
	}

	return memObj, nil
}

func GetDirInfo(fileptr *os.File) (*DirectoryPageV2, error) {
	byts, err := ReadNonPageFile(fileptr)
	if err != nil {
		return nil, fmt.Errorf("ReadNonPageFile failed: %w", err)
	}

	if len(byts) == 0 {
		return &DirectoryPageV2{make(map[PageID]*PageInfo)}, nil
	}

	dirPage, err := DecodeDirectory(byts)
	if err != nil {
		return nil, fmt.Errorf("DecodeDirectory failed: %w", err)
	}

	return dirPage, nil
}

func GetBpTree(fileptr *os.File) (*btree.BTree, error) {
	bpBytes, err := ReadNonPageFile(fileptr)
	if err != nil {
		return nil, fmt.Errorf("ReadNonPageFile failed: %w", err)
	}

	tree := NewTree(20)
	if len(bpBytes) > 0 {
		items, err := DecodeItems(bpBytes)
		if err != nil {
			return nil, fmt.Errorf("DecodeItems failed: %w", err)
		}

		for _, item := range items {
			tree.ReplaceOrInsert(item)
		}
	}

	return tree, nil
}

func (dm *DiskManagerV2) CreateTable(tableName string, info TableInfo) error {
	tablePath := filepath.Join(dm.DBdirectory, "Tables", tableName)

	dm.PageCatalog.Tables[tableName] = &info
	err := dm.UpdateCatalog()
	if err != nil {
		return fmt.Errorf("UpdateCatalog failed: %w", err)
	}

	err = os.Mkdir(tablePath, 0755)
	if err != nil && os.IsExist(err) {
		return fmt.Errorf("[%s] table already exists", tableName)
	}

	os.Create(filepath.Join(tablePath, tableName))
	os.Create(filepath.Join(tablePath, "directory_page"))
	os.Create(filepath.Join(tablePath, "bptree"))
	os.Create(filepath.Join(tablePath, "freeMem"))

	return nil
}

// space for optimization // could decode just the header
func FullTableScan(pc chan *PageV2, file *os.File, pageTable map[PageID]FrameID, bpm *BufferPoolManager) error {
	logger.Log.Info("FullTableScanNormalFiles")

	var offset int64
	for {
		buffer := make([]byte, PageSizeV2)
		_, err := file.ReadAt(buffer, int64(offset))
		if err != nil && err == io.EOF {
			if err == io.EOF {
				logger.Log.Info("FullTableScan (end of file)")
				break
			}

			return fmt.Errorf("reading at %d failed", offset)
		}

		page, err := DecodePageV2(buffer)
		if err != nil {
			return fmt.Errorf("DecodePageV2 failed: %w", err)
		}

		_, ok := pageTable[PageID(page.Header.ID)]
		if ok {
			logger.Log.Info("Skipped Page: ", page.Header.ID)
			offset += PageSizeV2
			continue
		}

		pc <- page

		logger.Log.WithField("PageId", page.Header.ID).Info("Page from disk")
		offset += PageSizeV2
	}

	return nil
}

type Chunk struct {
	Beggining int64
	End       int64
	NumPages  int
	Size      int64
}

func FullTableScanBigFiles(file *os.File, pageTable map[PageID]FrameID) ([]*PageV2, error) {
	logger.Log.Info("FullTableScanBigFiles")

	chunks := FileCreateChunks(file, PERCENTAGE)
	byteChan := make(chan []byte, BUFFER_SIZE)
	pageChan := make(chan *PageV2, BUFFER_SIZE)

	var wgReaders sync.WaitGroup
	for _, c := range chunks {
		wgReaders.Add(1)
		go func(chunk *Chunk) {
			defer wgReaders.Done()
			ReadWorker(file, chunk, byteChan, PAGES_PER_READ)
		}(c)
	}

	go func() {
		wgReaders.Wait()
		close(byteChan)
	}()

	var wgDecoders sync.WaitGroup
	for i := 0; i < NUM_DECODERS; i++ {
		wgDecoders.Add(1)
		go func() {
			defer wgDecoders.Done()
			DecoderWorker(byteChan, pageChan)
		}()
	}

	go func() {
		wgDecoders.Wait()
		close(pageChan)
	}()

	var pages []*PageV2
	for page := range pageChan {
		_, ok := pageTable[PageID(page.Header.ID)]
		if !ok {
			pages = append(pages, page)
		}
		logger.Log.Info("Skipped Page: ", page.Header.ID)
	}

	return pages, nil
}

func FileCreateChunks(file *os.File, percentage int) []*Chunk {
	stat, err := file.Stat()
	if err != nil {
		fmt.Println("FileCreateChunks (getting file stat):", err)
		return nil
	}

	fileSize := stat.Size()
	numPages := int(fileSize / PageSizeV2)
	perChunkPageNum := int(numPages * percentage / 100)
	blockSize := PageSizeV2 * int64(perChunkPageNum)

	var chunks []*Chunk
	for i := int64(0); i < fileSize; i += blockSize {
		end := i + blockSize
		if end > fileSize {
			end = fileSize
		}

		currBlockSize := end - i
		chunk := Chunk{
			Beggining: i,
			End:       end - 1,
			NumPages:  int(currBlockSize / PageSizeV2),
			Size:      currBlockSize,
		}

		chunks = append(chunks, &chunk)

		if end == fileSize {
			break
		}
	}

	return chunks
}

func ReadWorker(file *os.File, chunk *Chunk, byteChan chan []byte, pagesPerRead int) error {
	bufferSize := PageSizeV2 * pagesPerRead

	for offset := chunk.Beggining; offset <= chunk.End; {
		bytesToRead := bufferSize
		if offset+int64(bufferSize) > chunk.End+1 {
			bytesToRead = int(chunk.End - offset + 1)
		}

		buffer := make([]byte, bytesToRead)

		n, err := file.ReadAt(buffer, offset)
		if err == io.EOF && n < bytesToRead {
			buffer = buffer[:n]
		} else if err != nil {
			return fmt.Errorf("readWorker (reading file): %w", err)
		}

		offset += int64(n)
		byteChan <- buffer
	}

	return nil
}

func DecoderWorker(byteChan chan []byte, pageChan chan *PageV2) error {
	for buffer := range byteChan {
		numPages := len(buffer) / PageSizeV2

		for i := 0; i < numPages; i++ {
			start := i * PageSizeV2
			end := start + PageSizeV2
			if end > len(buffer) {
				end = len(buffer)
			}
			pageBuffer := buffer[start:end]
			page, err := DecodePageV2(pageBuffer)
			if err != nil {
				return fmt.Errorf("decodePageV2 failed: %v", err)
			}
			pageChan <- page
		}
	}

	return nil
}

func GetTablePagesFromDisk(pc chan *PageV2, dataFile *os.File, pageMemTable map[PageID]FrameID, bpm *BufferPoolManager) error {
	stat, _ := dataFile.Stat()
	size := stat.Size()

	if size >= MAX_FILE_SIZE {
		// return FullTableScanBigFiles(dataFile, pageMemTable)
	}
	return FullTableScan(pc, dataFile, pageMemTable, bpm)
}

func GetTableObj(tableName string, manager *DiskManagerV2) (*TableObj, error) {
	var tableObj *TableObj
	var err error

	tableObj, found := manager.TableObjs[tableName]
	if !found {
		tableObj, err = manager.InMemoryTableSetUp(tableName)
		if err != nil {
			return nil, fmt.Errorf("InMemoryTableSetUp failed: %w", err)
		}
	}

	return tableObj, err
}
