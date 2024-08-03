package storage

import (
	"fmt"
	"io"
	"os"
)

const PageSize = 4 * 1024

type DiskManagerV2 struct {
	DBdirectory string
	PageCatalog *Catalog
	FileCatalog *os.File
	TableObjs   map[TableName]*TableObj
}

type TableName string
type Catalog struct {
	Tables map[TableName]TableInfo
}

func NewDiskManagerV2(dbDirectory string) (*DiskManagerV2, error) {
	err := os.Mkdir(dbDirectory, 0755)
	if err != nil {
		return nil, fmt.Errorf("NewDiskManagerV2 (create db Dir error): %w", err)
	}

	err = os.Mkdir(dbDirectory+"/Tables", 0755)
	if err != nil {
		return nil, fmt.Errorf("NewDiskManagerV2 (create table dir error): %w", err)
	}

	catalogFilePtr, err := os.Create(dbDirectory + "/catalog")
	if err != nil {
		return nil, fmt.Errorf("NewDiskManagerV2 (create catalog file error): %w", err)
	}

	newCatalog := Catalog{Tables: make(map[TableName]TableInfo)}
	encodedCatalog, err := Encode(newCatalog)

	if err != nil {
		return nil, fmt.Errorf("NewDiskManagerV2: %w", err)
	}

	_, err = catalogFilePtr.Write(encodedCatalog)
	if err != nil {
		return nil, fmt.Errorf("NewDiskManagerV2 (catalog writing error): %w", err)
	}

	dm := DiskManagerV2{
		DBdirectory: dbDirectory,
		PageCatalog: &newCatalog,
		FileCatalog: catalogFilePtr,
		TableObjs:   make(map[TableName]*TableObj),
	}

	return &dm, nil
}

func (dm *DiskManagerV2) WriteToDisk(req DiskReq) error {
	tableInfo := dm.TableObjs[TableName(req.Page.TABLE)]
	offset, found := tableInfo.DirectoryPage.Mapping[req.Page.ID]

	if !found {
		pageOffset, err := dm.WritePageEOF(req, tableInfo)
		if err != nil {
			return fmt.Errorf("WriteToDisk: %w", err)
		}
		tableInfo.DirectoryPage.Mapping[req.Page.ID] = pageOffset
		err = dm.UpdateDirectoryPageDisk(tableInfo.DirectoryPage, tableInfo)
		if err != nil {
			return fmt.Errorf("WriteToDisk: %w", err)
		}
	} else {
		err := dm.WritePageBack(&req.Page, offset, tableInfo.DataFile)
		if err != nil {
			return fmt.Errorf("WriteToDisk: %w", err)
		}
	}

	return nil
}

func (dm *DiskManagerV2) UpdateDirectoryPageDisk(page *DirectoryPage, tableInfo *TableObj) error {
	encodedPage, err := Encode(page)
	if err != nil {
		return fmt.Errorf("UpdateDirectoryPageDisk: %w", err)
	}

	_, err = tableInfo.DirFile.WriteAt(encodedPage, 0)
	if err != nil {
		return fmt.Errorf("UpdateDirectoryPageDisk (Writing to Dir File): %w", err)
	}

	return nil
}

func FindAvailablePage(tablePtr *os.File, tableName string, row *Row) (*Page, error) {
	offset := 0
	page := Page{Rows: make(map[uint64]Row)}

	for {
		pageBytes := make([]byte, PageSize)
		_, err := tablePtr.ReadAt(pageBytes, int64(offset))
		if err != nil {
			if err == io.EOF {
				fmt.Println("FindAvailablePage (End of file reached, creating new page)")
				CreatePage(&page, row, tableName)
				return &page, nil
			}

			return nil, fmt.Errorf("FindAvailablePage(erro reading from file non-EOF): %w", err)
		}

		offset += PageSize
		err = DecodeV2(pageBytes, &page)
		if err != nil {
			return nil, fmt.Errorf("FindAvailablePage: %w", err)
		}

		cleanSize := getSizeOfIDAndRows(&page)

		if PageSize > cleanSize {
			page.TABLE = tableName
			row.ID = generateRandomID()
			page.Rows[row.ID] = *row
			break
		}

		page = Page{Rows: make(map[uint64]Row)}
	}

	return &page, nil
}
