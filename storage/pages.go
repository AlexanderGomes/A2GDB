package storage

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

type Row struct {
	ID     uint64
	Values map[string]string
}

type Page struct {
	ID       PageID
	TABLE    string
	Rows     map[uint64]Row
	IsDirty  bool
	IsPinned bool
}

type EncodablePage struct {
	ID   PageID
	Rows map[uint64]Row
}

type DirectoryPage struct {
	Mapping map[PageID]Offset
}

func (dm *DiskManagerV2) WritePageBack(page *Page, offset Offset, tableDataFile *os.File) error {
	cleanPage := EncodablePage{
		ID:   page.ID,
		Rows: page.Rows,
	}

	pageBytes, err := Encode(cleanPage)
	if err != nil {
		return fmt.Errorf("WritePageBack: %w", err)
	}

	paddingSize := PageSize - int64(len(pageBytes))
	buffer := append(pageBytes, make([]byte, paddingSize)...)

	_, err = tableDataFile.WriteAt(buffer, int64(offset))
	if err != nil {
		return fmt.Errorf("WritePageBack (failed writing page to disk): %w", err)
	}

	return nil
}

func (ds *DiskManagerV2) WritePageEOF(page *Page, tableInfo *TableObj) (Offset, error) {
	cleanPage := EncodablePage{
		ID:   page.ID,
		Rows: page.Rows,
	}

	encodedPage, err := Encode(cleanPage)
	if err != nil {
		return 0, fmt.Errorf("WritePageEOF: %w", err)
	}

	paddingSize := PageSize - int64(len(encodedPage))
	buffer := append(encodedPage, make([]byte, paddingSize)...)

	fileInfo, err := tableInfo.DataFile.Stat()
	if err != nil {
		return 0, fmt.Errorf("WritePageEOF (getting file info): %w", err)
	}

	offset := fileInfo.Size()

	n, err := tableInfo.DataFile.Write(buffer)
	if err != nil {
		return 0, fmt.Errorf("WritePageEOF (writing file to disk): %w", err)
	}

	if int64(n) != PageSize {
		return 0, fmt.Errorf("WritePageEOF (failed to write entire page to disk)")
	}

	return Offset(offset), nil
}


func ReadDirFile(dirFile *os.File) ([]byte, error) {
	var buffer bytes.Buffer

	tempBuffer := make([]byte, 1024)

	for {
		n, err := dirFile.Read(tempBuffer)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("ReadFile (error reading directory file): %w", err)
		}
		if n > 0 {
			buffer.Write(tempBuffer[:n])
		}
		if err == io.EOF {
			break
		}
	}

	return buffer.Bytes(), nil
}
