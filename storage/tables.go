package storage

import (
	"fmt"
	"github.com/google/btree"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

const (
	PERCENTAGE     = 25
	BUFFER_SIZE    = 20000
	NUM_DECODERS   = 20
	PAGES_PER_READ = 10
	MAX_FILE_SIZE  = 500 * 1024
)

type TableObj struct {
	DirectoryPage   *DirectoryPageV2
	DirFile         *os.File
	BpTree          *btree.BTree
	BpFile          *os.File
	DataFile        *os.File
	RearrangedPages []*RearrangedPage
}

type ColumnType struct {
	IsIndex bool
	Type    string
}

type TableInfo struct {
	Schema     map[string]ColumnType
	NumOfPages uint64
}

func (dm *DiskManagerV2) InMemoryTableSetUp(name TableName) (*TableObj, error) {
	dirFile, dirPage, err := GetDirInfo(dm.DBdirectory, string(name))
	if err != nil {
		return nil, fmt.Errorf("InMemoryTableSetUp: %w", err)
	}

	dataFile, err := GetDataFileInfo(dm.DBdirectory, string(name))
	if err != nil {
		return nil, fmt.Errorf("InMemoryTableSetUp: %w", err)
	}

	tree, bpFile, err := GetBpTree(dm.DBdirectory, string(name))
	if err != nil {
		return nil, fmt.Errorf("InMemoryTableSetUp: %w", err)
	}

	tableObj := &TableObj{
		DirectoryPage: dirPage,
		DataFile:      dataFile,
		DirFile:       dirFile,
		BpFile:        bpFile,
		BpTree:        tree,
	}

	err = GetRearrangedPages(tableObj)

	if err != nil {
		return nil, fmt.Errorf("InMemoryTableSetUp: %w", err)
	}

	dm.TableObjs[name] = tableObj
	return tableObj, nil
}

func GetRearrangedPages(tableObj *TableObj) error {
	pages, err := GetTablePages(tableObj.DataFile, nil)
	if err != nil {
		return fmt.Errorf("GetRearrangedPages: %w", err)
	}

	if len(pages) == 0 {
		return nil
	}

	dirMap := tableObj.DirectoryPage.Value
	for _, page := range pages {
		pageObj, ok := dirMap[PageID(page.Header.ID)]
		if !ok {
			return fmt.Errorf("GetRearrangedPages (pageObj not found)")
		}

		if !pageObj.Rearranged {
			continue
		}

		rearrengedObj := RearrangedPage{
			PageID: PageID(page.Header.ID),
			Offset: pageObj.Offset,
			Size:   pageObj.Size,
		}

		tableObj.RearrangedPages = append(tableObj.RearrangedPages, &rearrengedObj)
	}

	log.Printf("Total Rearranged pages: %d", len(tableObj.RearrangedPages))
	return nil
}

func GetBpTree(dbDirName, tableName string) (*btree.BTree, *os.File, error) {
	bpPath := filepath.Join(dbDirName, "Tables", tableName, "bptree")
	bpFile, err := os.OpenFile(bpPath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return nil, nil, fmt.Errorf("GetBpTree (error opening bp tree file): %w", err)
	}

	bpBytes, err := ReadNonPageFile(bpFile)
	if err != nil {
		return nil, nil, fmt.Errorf("GetBpTree: %w", err)
	}

	tree := NewTree(20)
	if len(bpBytes) > 0 {
		items, err := DecodeItems(bpBytes)
		if err != nil {
			return nil, nil, fmt.Errorf("GetBpTree: %w", err)
		}

		for _, item := range items {
			tree.ReplaceOrInsert(item)
		}
	}

	return tree, bpFile, nil
}

func GetDataFileInfo(dbDirName, tableName string) (*os.File, error) {
	tableDataPath := filepath.Join(dbDirName, "Tables", tableName, tableName)
	dataFile, err := os.OpenFile(tableDataPath, os.O_RDWR, 0666)
	if err != nil {
		return nil, fmt.Errorf("GetDataFileInfo (error opening data file): %w", err)
	}

	return dataFile, nil
}

func GetDirInfo(dbDirName, tableName string) (*os.File, *DirectoryPageV2, error) {
	dirFilePath := filepath.Join(dbDirName, "Tables", tableName, "directory_page")
	dirFile, err := os.OpenFile(dirFilePath, os.O_RDWR, 0666)
	if err != nil {
		return nil, nil, fmt.Errorf("GetDirInfo (error opening directory_page file): %w", err)
	}

	byts, err := ReadNonPageFile(dirFile)
	if err != nil {
		return nil, nil, fmt.Errorf("GetDirInfo (error reading Dir File): %w", err)
	}

	if len(byts) == 0 {
		return dirFile, &DirectoryPageV2{make(map[PageID]*PageInfo)}, nil
	}

	dirPage, err := DecodeDirectory(byts)
	if err != nil {
		return nil, nil, fmt.Errorf("GetDirInfo (decoding): %w", err)
	}

	return dirFile, dirPage, nil
}

func (dm *DiskManagerV2) CreateTable(name TableName, info TableInfo) error {
	tablePath := filepath.Join(dm.DBdirectory, "Tables", string(name))

	dm.PageCatalog.Tables[name] = info
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
