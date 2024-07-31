package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const PageSize = 4 * 1024

type EncodablePage struct {
	ID   PageID
	Rows map[uint64]Row
}

type DiskManagerV2 struct {
	DBdirectory string
	PageCatalog *Catalog
	FileCatalog *os.File
	TableObjs   map[TableName]*TableObj
}

type TableObj struct {
	DirectoryPage *DirectoryPage
	DataFile      *os.File
	DirFile       *os.File
}

type TableName string
type Catalog struct {
	Tables map[TableName]TableInfo
}

type TableInfo struct {
}

func NewDiskManagerV2(dbDirectory string) (*DiskManagerV2, error) {

	err := os.Mkdir(dbDirectory, 0755)
	if err != nil {
		return nil, fmt.Errorf("create db Dir error: %w", err)
	}

	err = os.Mkdir(dbDirectory+"/Tables", 0755)
	if err != nil {
		return nil, fmt.Errorf("create table dir error: %w", err)
	}

	catalogFilePtr, err := os.Create(dbDirectory + "/catalog")
	if err != nil {
		return nil, fmt.Errorf("create catalog file error: %w", err)
	}

	newCatalog := Catalog{Tables: make(map[TableName]TableInfo)}
	encodedCatalog, _ := Encode(newCatalog)

	_, err = catalogFilePtr.Write(encodedCatalog)
	if err != nil {
		return nil, fmt.Errorf("catalog writing error: %w", err)
	}

	dm := DiskManagerV2{
		DBdirectory: dbDirectory,
		PageCatalog: &newCatalog,
		FileCatalog: catalogFilePtr,
		TableObjs:   make(map[TableName]*TableObj),
	}

	return &dm, nil
}

func (dm *DiskManagerV2) InMemoryTableSetUp(name TableName) (*TableObj, error) {
	dirFilePath := filepath.Join(dm.DBdirectory, "Tables", string(name), "directory_page")
	dirFile, err := os.OpenFile(dirFilePath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("error opening directory_page file %w", err)
	}

	//#TODO - read the whole file
	byts := make([]byte, 1024)
	_, err = dirFile.Read(byts)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("error reading directory file %w", err)
	}

	dirPage := DirectoryPage{}
	if err := DecodeV2(byts, &dirPage); err != nil {
		return nil, fmt.Errorf("error decoding directory page %w", err)
	}

	tableDataPath := filepath.Join(dm.DBdirectory, "Tables", string(name), string(name))
	dataFile, err := os.OpenFile(tableDataPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("error opening data file %w", err)
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
	tablePath := filepath.Join(dm.DBdirectory, "/Tables", string(name))

	// # update catalog
	dm.PageCatalog.Tables[name] = info

	// # create table directory
	err := os.Mkdir(tablePath, 0755)
	if err != nil {
		return fmt.Errorf("create table dir error: %w", err)
	}

	// # create the table file
	_, err = os.Create(filepath.Join(tablePath, string(name)))
	if err != nil {
		return fmt.Errorf("create table file error: %w", err)
	}

	// # create directory page file for table file
	dirPage := DirectoryPage{Mapping: map[PageID]Offset{}}

	bytes, _ := Encode(dirPage)

	dir, err := os.Create(filepath.Join(tablePath, "directory_page"))
	if err != nil {
		return fmt.Errorf("create directory_page error: %w", err)
	}

	dir.WriteAt(bytes, 0)

	return nil
}

func (dm *DiskManagerV2) WriteToDisk(req DiskReq) error {
	tableInfo := dm.TableObjs[TableName(req.Page.TABLE)]
	offset, found := tableInfo.DirectoryPage.Mapping[req.Page.ID]

	if !found {
		pageOffset, err := dm.CreatePage(req, tableInfo)
		if err != nil {
			return fmt.Errorf("error creating page: %w", err)
		}
		tableInfo.DirectoryPage.Mapping[req.Page.ID] = pageOffset
		dm.UpdateDirectoryPageDisk(tableInfo.DirectoryPage, tableInfo)
	} else {
		dm.WritePage(req.Page, offset, tableInfo)
	}

	return nil
}

func (ds *DiskManagerV2) CreatePage(req DiskReq, tableInfo *TableObj) (Offset, error) {
	cleanPage := EncodablePage{
		ID:   req.Page.ID,
		Rows: req.Page.Rows,
	}

	encodedPage, err := Encode(cleanPage)
	if err != nil {
		return 0, err
	}

	paddingSize := PageSize - len(encodedPage)

	buffer := append(encodedPage, make([]byte, paddingSize)...)

	fileInfo, err := tableInfo.DataFile.Stat()
	if err != nil {
		return 0, fmt.Errorf("error fileStat; %w", err)
	}

	offset := fileInfo.Size()

	n, err := tableInfo.DataFile.Write(buffer)
	if err != nil {
		return 0, fmt.Errorf("error writting file; %w", err)
	}

	if n != PageSize {
		return 0, fmt.Errorf("failed to write entire page to disk")
	}

	return Offset(offset), nil
}

func (dm *DiskManagerV2) WritePage(page Page, offset Offset, tableInfo *TableObj) error {
	cleanPage := EncodablePage{
		ID:   page.ID,
		Rows: page.Rows,
	}

	pageBytes, _ := Encode(cleanPage)

	if len(pageBytes) > PageSize {
		pageBytes = pageBytes[:PageSize]
	}

	_, err := tableInfo.DataFile.WriteAt(pageBytes, int64(offset))
	if err != nil {
		return err
	}

	return nil
}

func (dm *DiskManagerV2) UpdateDirectoryPageDisk(page *DirectoryPage, tableInfo *TableObj) error {
	encodedPage, err := Encode(page)
	if err != nil {
		return err
	}

	_, err = tableInfo.DirFile.WriteAt(encodedPage, 0)
	if err != nil {
		return err
	}

	return nil
}

func Encode(page interface{}) ([]byte, error) {
	encoded, err := json.Marshal(page)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func (dm *DiskManagerV2) WriteCatalog() {
	bytes, _ := Encode(dm.PageCatalog)
	dm.FileCatalog.WriteAt(bytes, 0)
}

func DecodeV2(page []byte, dst interface{}) error {
	endIndex := bytes.IndexByte(page, 0)

	err := json.Unmarshal(page[:endIndex], dst)
	if err != nil {
		return err
	}

	return nil
}
