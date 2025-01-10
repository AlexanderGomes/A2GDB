package storage

import (
	"a2gdb/logger"
	"fmt"
	"io"
	"os"
)

func GetTablePagesFromDiskTest(dataFile *os.File) ([]*PageV2, error) {
	stat, _ := dataFile.Stat()
	size := stat.Size()

	if size >= MAX_FILE_SIZE {
		// return FullTableScanBigFiles(dataFile, pageMemTable)
	}
	return FullTableScanTest(dataFile)
}

func FullTableScanTest(file *os.File) ([]*PageV2, error) {
	logger.Log.Info("FullTableScanNormalFiles")

	var offset int64
	pageSlice := []*PageV2{}

	for {
		buffer := make([]byte, PageSizeV2)
		_, err := file.ReadAt(buffer, int64(offset))
		if err != nil && err == io.EOF {
			if err == io.EOF {
				logger.Log.Info("FullTableScan (end of file)")
				break
			}

			return nil, fmt.Errorf("reading at %d failed", offset)
		}

		page, err := DecodePageV2(buffer)
		if err != nil {
			return []*PageV2{}, fmt.Errorf("DecodePageV2 failed: %w", err)
		}

		pageSlice = append(pageSlice, page)
		offset += PageSizeV2
	}

	return pageSlice, nil
}
