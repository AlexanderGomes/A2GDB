package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	offset := 0
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
