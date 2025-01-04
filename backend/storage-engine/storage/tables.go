package storage

import (
	"fmt"
	"io"
	"log"
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
	MAX_FILE_SIZE  = 1 * 1024 * 1024
)

type TableObj struct {
	DirectoryPage *DirectoryPageV2
	BpTree        *btree.BTree
	Memory        map[uint16][]*FreeSpace
	DirFile       *os.File
	BpFile        *os.File
	DataFile      *os.File
	MemFile       *os.File
}

type FreeSpace struct {
	PageID      PageID
	FreeMemory  uint16
	TempPagePtr *PageV2
}

func (dm *DiskManagerV2) InMemoryTableSetUp(tableName string) (*TableObj, error) {
	dirObj, dirFilePtr := GetNonpageFile(dm.DBdirectory, tableName, "directory_page")
	bptreeObj, bptreeFilePtr := GetNonpageFile(dm.DBdirectory, tableName, "bptree")
	memObj, memFilePtr := GetNonpageFile(dm.DBdirectory, tableName, "freeMem")
	_, dataFilePtr := GetNonpageFile(dm.DBdirectory, tableName, tableName)

	tableObj := &TableObj{
		DirectoryPage: dirObj.(*DirectoryPageV2),
		BpTree:        bptreeObj.(*btree.BTree),
		Memory:        memObj.(map[uint16][]*FreeSpace),
		DataFile:      dataFilePtr,
		DirFile:       dirFilePtr,
		BpFile:        bptreeFilePtr,
		MemFile:       memFilePtr,
	}

	dm.TableObjs[TableName(tableName)] = tableObj
	return tableObj, nil
}

func GetNonpageFile(dbDirectory, tableName, fileName string) (interface{}, *os.File) {
	var object interface{}

	filePath := filepath.Join(dbDirectory, "Tables", tableName, fileName)
	filePtr, err := os.OpenFile(filePath, os.O_RDWR, 0666)
	if err != nil {
		log.Fatalf("opening non page file failed: %s", err)
	}

	switch fileName {
	case "bptree":
		object, err = GetBpTree(filePtr)
		if err != nil {
			log.Fatalf("getting bp data failed: %s", err)
		}
	case "directory_page":
		object, err = GetDirInfo(filePtr)
		if err != nil {
			log.Fatalf("getting directory data failed: %s", err)
		}
	case tableName:
		return nil, filePtr
	case "freeMem":
		object, err = GetMemInfo(filePtr)
		if err != nil {
			log.Fatalf("getting mem obj failed: %s", err)
		}
	}

	return object, filePtr
}

func GetMemInfo(fileptr *os.File) (map[uint16][]*FreeSpace, error) {
	byts, err := ReadNonPageFile(fileptr)
	if err != nil {
		return nil, fmt.Errorf("GetDirInfo (error reading Dir File): %w", err)
	}

	if len(byts) == 0 {
		return make(map[uint16][]*FreeSpace), nil
	}

	memObj, err := DecodeMemObj(byts)
	if err != nil {
		return nil, fmt.Errorf("GetDirInfo (decoding): %w", err)
	}

	return memObj, nil
}

func GetDirInfo(fileptr *os.File) (*DirectoryPageV2, error) {
	byts, err := ReadNonPageFile(fileptr)
	if err != nil {
		return nil, fmt.Errorf("GetDirInfo (error reading Dir File): %w", err)
	}

	if len(byts) == 0 {
		return &DirectoryPageV2{make(map[PageID]*PageInfo)}, nil
	}

	dirPage, err := DecodeDirectory(byts)
	if err != nil {
		return nil, fmt.Errorf("GetDirInfo (decoding): %w", err)
	}

	return dirPage, nil
}

func GetBpTree(fileptr *os.File) (*btree.BTree, error) {
	bpBytes, err := ReadNonPageFile(fileptr)
	if err != nil {
		return nil, fmt.Errorf("GetBpTree: %w", err)
	}

	tree := NewTree(20)
	if len(bpBytes) > 0 {
		items, err := DecodeItems(bpBytes)
		if err != nil {
			return nil, fmt.Errorf("GetBpTree: %w", err)
		}

		for _, item := range items {
			tree.ReplaceOrInsert(item)
		}
	}

	return tree, nil
}

func (dm *DiskManagerV2) CreateTable(name TableName, info TableInfo) error {
	tablePath := filepath.Join(dm.DBdirectory, "Tables", string(name))

	dm.PageCatalog.Tables[name] = &info
	err := dm.UpdateCatalog()
	if err != nil {
		return fmt.Errorf("CreateTable: %w", err)
	}

	err = os.Mkdir(tablePath, 0755)
	if err != nil && os.IsExist(err) {
		return nil
	}

	os.Create(filepath.Join(tablePath, string(name)))
	os.Create(filepath.Join(tablePath, "directory_page"))
	os.Create(filepath.Join(tablePath, "bptree"))
	os.Create(filepath.Join(tablePath, "freeMem"))

	return nil
}

func FullTableScan(file *os.File) ([]*PageV2, error) {
	var offset int64
	pageSlice := []*PageV2{}

	for {
		buffer := make([]byte, PageSizeV2)
		_, err := file.ReadAt(buffer, int64(offset))
		if err != nil && err == io.EOF {
			fmt.Println("FullTableScan (end of file)")
			break
		}

		page, err := DecodePageV2(buffer)
		if err != nil {
			return []*PageV2{}, fmt.Errorf("FullTableScan: %w", err)
		}

		pageSlice = append(pageSlice, page)
		offset += PageSizeV2
	}

	return pageSlice, nil
}

type Chunk struct {
	Beggining int64
	End       int64
	NumPages  int
	Size      int64
}

func FullTableScanBigFiles(file *os.File) ([]*PageV2, error) {
	log.Println("FullTableScanBigFiles")

	chunks := FileCreateChunks(file, PERCENTAGE)
	byteChan := make(chan []byte, BUFFER_SIZE)
	pageChan := make(chan *PageV2, BUFFER_SIZE)

	var wgManagers sync.WaitGroup
	for _, c := range chunks {
		wgManagers.Add(1)
		go func(chunk *Chunk) {
			defer wgManagers.Done()
			ReadWorker(file, chunk, byteChan, PAGES_PER_READ)
		}(c)
	}

	go func() {
		wgManagers.Wait()
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
		pages = append(pages, page)
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

func ReadWorker(file *os.File, chunk *Chunk, byteChan chan []byte, pagesPerRead int) {
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
			fmt.Println("ReadWorker (reading file):", err)
			return
		}

		offset += int64(n)
		byteChan <- buffer
	}
}

func DecoderWorker(byteChan chan []byte, pageChan chan *PageV2) {
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
				fmt.Printf("Decoder: %v\n", err)
				continue
			}
			pageChan <- page
		}
	}
}

func GetTablePages(dataFile *os.File, offset *Offset) ([]*PageV2, error) {
	stat, _ := dataFile.Stat()
	size := stat.Size()

	if offset == nil {
		if size >= MAX_FILE_SIZE {
			return FullTableScanBigFiles(dataFile)
		}
		return FullTableScan(dataFile)
	}

	bytes, err := ReadPageAtOffset(dataFile, *offset)
	if err != nil {
		return nil, fmt.Errorf("getTablePages: %w", err)
	}

	page, err := DecodePageV2(bytes)
	if err != nil {
		return nil, fmt.Errorf("getTablePages: %w", err)
	}

	log.Println("Index Scan")
	return []*PageV2{page}, nil
}

func GetTableObj(tableName string, manager *DiskManagerV2) (*TableObj, error) {
	var tableObj *TableObj
	var err error

	tableObj, found := manager.TableObjs[TableName(tableName)]
	if !found {
		tableObj, err = manager.InMemoryTableSetUp(tableName)
		if err != nil {
			return nil, fmt.Errorf("GetTable: %w", err)
		}
	}

	return tableObj, err
}
