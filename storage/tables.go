package storage

import (
	"fmt"

	"io"
	"math"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/btree"
)

const (
	PERCENTAGE   = 10
	BUFFER_SIZE  = 20000
	NUM_DECODERS = 1000
)

type TableObj struct {
	DirectoryPage *DirectoryPageV2
	DirFile       *os.File
	BpTree        *btree.BTree
	BpFile        *os.File
	DataFile      *os.File
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
	dirFilePath := filepath.Join(dm.DBdirectory, "Tables", string(name), "directory_page")

	dirFile, err := os.OpenFile(dirFilePath, os.O_RDWR, 0777)
	if err != nil {
		return nil, fmt.Errorf("InMemoryTableSetUp (error opening directory_page file): %w", err)
	}

	byts, err := ReadNonPageFile(dirFile)
	if err != nil {
		return nil, fmt.Errorf("InMemoryTableSetUp (error reading Dir File): %w", err)
	}

	dirPage, err := DecodeDirectory(byts)
	if err != nil {
		return nil, fmt.Errorf("InMemoryTableSetUp: %w", err)
	}

	tableDataPath := filepath.Join(dm.DBdirectory, "Tables", string(name), string(name))
	dataFile, err := os.OpenFile(tableDataPath, os.O_RDWR, 0777)
	if err != nil {
		return nil, fmt.Errorf("InMemoryTableSetUp (error opening data file): %w", err)
	}

	bpPath := filepath.Join(dm.DBdirectory, "Tables", string(name), "bptree")
	bpFile, err := os.OpenFile(bpPath, os.O_RDWR|os.O_CREATE, 0777)
	if err != nil {
		return nil, fmt.Errorf("InMemoryTableSetUp (error opening bp tree file): %w", err)
	}

	bpBytes, err := ReadNonPageFile(bpFile)
	if err != nil {
		return nil, fmt.Errorf("InMemoryTableSetUp: %w", err)
	}

	tree := NewTree(20)
	if len(bpBytes) > 0 {
		items, err := DecodeItems(bpBytes)
		if err != nil {
			return nil, fmt.Errorf("InMemoryTableSetUp: %w", err)
		}

		for _, item := range items {
			tree.ReplaceOrInsert(item)
		}
	}

	tableObj := &TableObj{
		DirectoryPage: dirPage,
		DataFile:      dataFile,
		DirFile:       dirFile,
		BpFile:        bpFile,
		BpTree:        tree,
	}

	dm.TableObjs[name] = tableObj

	return tableObj, nil
}

func (dm *DiskManagerV2) CreateTable(name TableName, info TableInfo) error {
	tablePath := filepath.Join(dm.DBdirectory, "Tables", string(name))

	dm.PageCatalog.Tables[name] = info
	err := dm.UpdateCatalog()
	if err != nil {
		return fmt.Errorf("CreateTable: %w", err)
	}

	err = os.Mkdir(tablePath, 0755)
	if err != nil {
		return fmt.Errorf("CreateTable (create table dir error): %w", err)
	}

	_, err = os.Create(filepath.Join(tablePath, string(name)))
	if err != nil {
		return fmt.Errorf("CreateTable (create table file error): %w", err)
	}

	dirPage := DirectoryPageV2{}
	bytes, err := EncodeDirectory(&dirPage)
	if err != nil {
		return fmt.Errorf("CreateTable: %w", err)
	}

	dir, err := os.Create(filepath.Join(tablePath, "directory_page"))
	if err != nil {
		return fmt.Errorf("CreateTable (create directory_page error): %w", err)
	}

	_, err = dir.WriteAt(bytes, 0)
	if err != nil {
		return fmt.Errorf("CreateTable (error writing to disk): %w", err)
	}

	_, err = os.Create(filepath.Join(tablePath, string(name), "bptree"))
	if err != nil {
		return fmt.Errorf("CreateTable (create bptree file error): %w", err)
	}

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
	chunks := FileCreateChunks(file, PERCENTAGE)
	byteChan := make(chan []byte, BUFFER_SIZE)
	pageChan := make(chan *PageV2, BUFFER_SIZE)

	var pages []*PageV2

	var wgManagers sync.WaitGroup
	for _, c := range chunks {
		wgManagers.Add(1)
		go func(chunk *Chunk) {
			defer wgManagers.Done()
			ReadersManager(file, chunk, byteChan)
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

	for page := range pageChan {
		pages = append(pages, page)
	}

	return pages, nil
}

func DecoderWorker(byteChan chan []byte, pageChan chan *PageV2) {
	for pageByte := range byteChan {
		page, err := DecodePageV2(pageByte)
		if err != nil {
			fmt.Println("Decoder: %w", err)
		}
		pageChan <- page
	}
	fmt.Println("Decoder has finished processing all data and byteChan is closed")
}

func ReadersManager(file *os.File, chunk *Chunk, byteChan chan []byte) {
	chunks := ChunkCreateChunks(chunk)

	var wg WrapperWaitGroup

	for _, c := range chunks {
		wg.Add(1)
		go func(chunk *Chunk) {
			defer wg.Done()
			ReadWoker(file, chunk, byteChan)
		}(c)
	}

	wg.Wait()
}

func ReadWoker(file *os.File, chunk *Chunk, byteChan chan []byte) {
	for offset := chunk.Beggining; offset < chunk.End; offset += PageSizeV2 {
		buffer := make([]byte, PageSizeV2)

		_, err := file.ReadAt(buffer, offset)
		if err != nil {
			fmt.Println("ReadWoker: %w", err)
			break
		}
		byteChan <- buffer
	}
}

func ChunkCreateChunks(chunk *Chunk) []*Chunk {
	numPagesFloat := float64(chunk.NumPages)
	percentageFloat := float64(PERCENTAGE)

	perChunkPageNum := math.Ceil(numPagesFloat * percentageFloat / 100)
	blockSize := PageSizeV2 * int64(perChunkPageNum)

	var chunks []*Chunk
	for i := int64(chunk.Beggining); i < chunk.End; i += int64(blockSize) {
		end := i + blockSize - 1
		currBlockSize := blockSize

		isEndBlock := end > chunk.End
		if isEndBlock {
			end = chunk.End - 1
			currBlockSize = chunk.End - i + 1
		}

		chunk := Chunk{
			Beggining: i,
			End:       end,
			NumPages:  int(currBlockSize / PageSizeV2),
			Size:      currBlockSize,
		}

		chunks = append(chunks, &chunk)
	}

	return chunks
}

func FileCreateChunks(file *os.File, percentage int) []*Chunk {
	stat, err := file.Stat()
	if err != nil {
		fmt.Println("FullTableScanBigFiles (getting file stat)")
		return nil
	}

	fileSize := stat.Size()

	numPages := int(fileSize / PageSizeV2)
	perChunkPageNum := int(numPages * percentage / 100)
	blockSize := PageSizeV2 * int64(perChunkPageNum)

	var chunks []*Chunk
	for i := int64(0); i < fileSize; i += int64(blockSize) {
		end := i + blockSize - 1
		currBlockSize := blockSize

		isEndBlock := end > fileSize
		if isEndBlock {
			end = fileSize - 1
			currBlockSize = end - i + 1
		}

		chunk := Chunk{
			Beggining: i,
			End:       end,
			NumPages:  int(currBlockSize / PageSizeV2),
			Size:      currBlockSize,
		}
		chunks = append(chunks, &chunk)
	}

	return chunks
}
