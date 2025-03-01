package engines

import (
	"a2gdb/logger"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/btree"
)

const (
	PERCENTAGE     = 20
	BUFFER_SIZE    = 25
	NUM_DECODERS   = 10
	PAGES_PER_READ = 20
	MAX_FILE_SIZE  = 20 * 1024
)

type TableObj struct {
	DirectoryPage *DirectoryPageV2
	Memory        map[uint16][]*FreeSpace
	DirFile       *os.File
	DataFile      *os.File
	MemFile       *os.File
	TableName     string
	Mu            sync.RWMutex
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
		Memory:        memObj.(map[uint16][]*FreeSpace),
		DataFile:      dataFilePtr,
		DirFile:       dirFilePtr,
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
		return &DirectoryPageV2{Value: make(map[PageID]*PageInfo), Mu: sync.RWMutex{}}, nil
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

	err := os.Mkdir(tablePath, 0755)
	if err != nil && os.IsExist(err) {
		return fmt.Errorf("[%s] table already exists", tableName)
	}

	dm.PageCatalog.Tables[tableName] = &info
	err = dm.UpdateCatalog()
	if err != nil {
		return fmt.Errorf("UpdateCatalog failed: %w", err)
	}

	os.Create(filepath.Join(tablePath, tableName))
	os.Create(filepath.Join(tablePath, "directory_page"))
	os.Create(filepath.Join(tablePath, "freeMem"))

	return nil
}

// space for optimization // could decode just the header
func FullTableScan(outerCtx, innerCtx context.Context, pc chan *PageV2, file *os.File, pageTable map[PageID]FrameID, tp uint64) error {
	var offset int64
	var pageCount uint64

	for {
		select {
		case <-outerCtx.Done():
			return outerCtx.Err()
		case <-innerCtx.Done():
			return innerCtx.Err()
		default:
			if pageCount == tp {
				return nil
			}

			buffer := make([]byte, PageSizeV2)
			_, err := file.ReadAt(buffer, int64(offset))
			if err != nil && err == io.EOF {
				if err == io.EOF {
					logger.Log.Info("FullTableScanNormalFiles (end of file)")
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
			pageCount++
		}
	}
}

type Chunk struct {
	Beggining int64
	End       int64
	NumPages  int
	Size      int64
}

// needs page counter
func FullTableScanBigFiles(outerCtx context.Context, pc chan *PageV2, file *os.File, pageTable map[PageID]FrameID, tp uint64) error {
	logger.Log.Info("FullTableScanBigFiles")

	chunks := FileCreateChunks(file, PERCENTAGE)
	byteChan := make(chan []byte, BUFFER_SIZE)
	errChan := make(chan error, 4)

	innerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wgReaders sync.WaitGroup
	for _, c := range chunks {
		wgReaders.Add(1)
		go func(chunk *Chunk) {
			defer wgReaders.Done()
			if err := ReadWorker(innerCtx, outerCtx, file, chunk, byteChan, PAGES_PER_READ); err != nil {
				errChan <- err
				cancel()
			}
		}(c)
	}

	go func() {
		wgReaders.Wait()
		close(byteChan)
	}()

	var wgDecoders sync.WaitGroup
	for range NUM_DECODERS {
		wgDecoders.Add(1)
		go func() {
			defer wgDecoders.Done()
			if err := DecoderWorker(innerCtx, outerCtx, byteChan, pc, pageTable); err != nil {
				errChan <- err
				cancel()
			}
		}()
	}

	wgDecoders.Wait()
	close(errChan)

	select {
	case err := <-errChan:
		return err
	default:
		return nil
	}
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

func ReadWorker(innerCtx, outerCtx context.Context, file *os.File, chunk *Chunk, byteChan chan []byte, pagesPerRead int) error {
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

		select {
		case <-innerCtx.Done():
			return innerCtx.Err()
		case <-outerCtx.Done():
			return outerCtx.Err()
		case byteChan <- buffer:
		}
	}

	return nil
}

func DecoderWorker(innerCtx, outerCtx context.Context, byteChan chan []byte, pageChan chan *PageV2, pageTable map[PageID]FrameID) error {
	for {
		select {
		case <-innerCtx.Done():
			return innerCtx.Err()
		case <-outerCtx.Done():
			return outerCtx.Err()
		case buffer, ok := <-byteChan:
			if !ok {
				return nil
			}

			numPages := len(buffer) / PageSizeV2

			for i := range numPages {
				start := i * PageSizeV2
				end := min(start+PageSizeV2, len(buffer))
				pageBuffer := buffer[start:end]
				page, err := DecodePageV2(pageBuffer)
				if err != nil {
					return fmt.Errorf("decodePageV2 failed: %v", err)
				}

				_, ok := pageTable[PageID(page.Header.ID)]
				if !ok {
					pageChan <- page
				}
			}
		}
	}
}

func GetTablePagesFromDisk(outerCtx, innerCtx context.Context, pc chan *PageV2, tableObj *TableObj, pageMemTable map[PageID]FrameID, totalPages uint64) error {
	// stat, _ := tableObj.DataFile.Stat()
	// size := stat.Size()

	// if size >= MAX_FILE_SIZE {
	// 	return FullTableScanBigFiles(ctx, pc, tableObj.DataFile, pageMemTable, totalPages)
	// }
	return FullTableScan(outerCtx, innerCtx, pc, tableObj.DataFile, pageMemTable, totalPages)
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
