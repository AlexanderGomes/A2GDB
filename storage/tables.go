package storage

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
)

const (
	PERCENTAGE   = 10
	BUFFER_SIZE  = 20000
	NUM_DECODERS = 1000
)

type TableObj struct {
	DirectoryPage *DirectoryPage
	DataFile      *os.File
	DirFile       *os.File
}

type TableInfo struct {
}

func (dm *DiskManagerV2) InMemoryTableSetUp(name TableName) (*TableObj, error) {
	dirFilePath := filepath.Join(dm.DBdirectory, "Tables", string(name), "directory_page")

	dirFile, err := os.OpenFile(dirFilePath, os.O_RDWR|os.O_CREATE, 0777)
	if err != nil {
		return nil, fmt.Errorf("InMemoryTableSetUp (error opening directory_page file): %w", err)
	}

	byts, err := ReadDirFile(dirFile)
	if err != nil {
		return nil, fmt.Errorf("InMemoryTableSetUp (error reading Dir File): %w", err)
	}

	dirPage := DirectoryPage{}
	if err := DecodeV2(byts, &dirPage); err != nil {
		return nil, fmt.Errorf("InMemoryTableSetUp: %w", err)
	}

	tableDataPath := filepath.Join(dm.DBdirectory, "Tables", string(name), string(name))
	dataFile, err := os.OpenFile(tableDataPath, os.O_RDWR|os.O_CREATE, 0777)
	if err != nil {
		return nil, fmt.Errorf("InMemoryTableSetUp (error opening data file): %w", err)
	}

	tableObj := &TableObj{
		DirectoryPage: &dirPage,
		DataFile:      dataFile,
		DirFile:       dirFile,
	}

	dm.TableObjs[name] = tableObj

	return tableObj, nil
}

func (dm *DiskManagerV2) CreateTable(name TableName, info TableInfo) error {
	tablePath := filepath.Join(dm.DBdirectory, "Tables", string(name))

	// # update catalog
	dm.PageCatalog.Tables[name] = info
	err := dm.UpdateCatalog()
	if err != nil {
		return fmt.Errorf("CreateTable: %w", err)
	}

	// # create table directory
	err = os.Mkdir(tablePath, 0755)
	if err != nil {
		return fmt.Errorf("CreateTable (create table dir error): %w", err)
	}

	// # create the table file
	_, err = os.Create(filepath.Join(tablePath, string(name)))
	if err != nil {
		return fmt.Errorf("CreateTable (create table file error): %w", err)
	}

	// # create directory page file for table file
	dirPage := DirectoryPage{Mapping: map[PageID]Offset{}}
	bytes, err := Encode(dirPage)
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

	return nil
}

func FullTableScan(file *os.File) ([]*Page, error) {
	var offset int64
	pageSlice := []*Page{}

	for {
		page := Page{}
		buffer := make([]byte, PageSize)
		_, err := file.ReadAt(buffer, int64(offset))
		if err != nil && err == io.EOF {
			fmt.Println("FullTableScan (end of file, processing pages...)")
			break
		}

		err = DecodeV2(buffer, &page)
		if err != nil {
			return []*Page{}, fmt.Errorf("FullTableScan: %w", err)
		}

		pageSlice = append(pageSlice, &page)
		offset += PageSize
	}

	return pageSlice, nil
}

type Chunk struct {
	Beggining int64
	End       int64
	NumPages  int
	Size      int64
}

func FullTableScanBigFiles(file *os.File) ([]*Page, error) {
	chunks := FileCreateChunks(file, PERCENTAGE)
	byteChan := make(chan []byte, BUFFER_SIZE)
	pageChan := make(chan *Page, BUFFER_SIZE)

	var pages []*Page

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
			Decoder(byteChan, pageChan)
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

func Decoder(byteChan chan []byte, pageChan chan *Page) {
	for pageByte := range byteChan {
		page := Page{}
		err := DecodeV2(pageByte, &page)
		if err != nil {
			fmt.Println("Decoder: %w", err)
		}
		pageChan <- &page
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
	for offset := chunk.Beggining; offset < chunk.End; offset += PageSize {
		buffer := make([]byte, PageSize)

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
	blockSize := PageSize * int64(perChunkPageNum)

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
			NumPages:  int(currBlockSize / PageSize),
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

	numPages := int(fileSize / PageSize)
	fmt.Printf("file num pages (int): %d\n", fileSize/PageSize)
	perChunkPageNum := int(numPages * percentage / 100)
	blockSize := PageSize * int64(perChunkPageNum)

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
			NumPages:  int(currBlockSize / PageSize),
			Size:      currBlockSize,
		}

		fmt.Printf("block num pages: %d\n", chunk.NumPages)
		chunks = append(chunks, &chunk)
	}

	return chunks
}
