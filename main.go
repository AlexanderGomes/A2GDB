package main

import (
	queryengine "disk-db/query-engine"
	"disk-db/storage"
	"fmt"
	"log"
	"os"
)

const (
	replacerFrequency = 2
	dirName           = "A2G_DB"
	path              = "A2G_DB/Tables/Company/Company"
	NumPages          = (100 * 1024 * 1024 * 1024) / storage.PageSize
)

func main() {
	page := storage.CreatePageV2()
	rows := []byte("name:alex,age:12,wife:malavika,school:pinole,name:alex,age:12,wife:malavika,school:pinole")
	tuple := storage.Tuple{
		Data: rows,
		Header: storage.TupleHeader{
			Length: uint16(len(rows)),
			Flags:  7,
		},
	}

	bts := storage.SerializeTuple(tuple)

	for i := 0; i < 30000; i++ {
		err := page.AddTuple(bts)

		if err != nil {
			fmt.Println(err)
		}
	}

}

func CreateTestFile() {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0777)
	if err != nil {
		fmt.Println("Openfile: %w", err)
	}

	for i := int64(0); i < NumPages; i++ {
		page := storage.Page{
			ID:       storage.PageID(i),
			TABLE:    "TestTable",
			Rows:     make(map[uint64]storage.Row),
			IsDirty:  false,
			IsPinned: false,
		}

		for j := 0; j < 38; j++ {
			page.Rows[uint64(j)] = storage.Row{
				ID:     uint64(j),
				Values: map[string]string{"column1": "value1", "column2": "value2", "column12": "value1", "column21": "value2"},
			}
		}

		encodedPage, err := storage.Encode(page)
		if err != nil {
			fmt.Println("Error encoding page:", err)
			return
		}

		paddingSize := int(storage.PageSize) - len(encodedPage)
		if paddingSize < 0 {
			// Handle case where encodedPage is larger than PageSize
			paddingSize = 0
		}

		buffer := append(encodedPage, make([]byte, paddingSize)...)

		_, err = file.Write(buffer)
		if err != nil {
			fmt.Println("Error writing page to file:", err)
			return
		}
	}

}

func InitDatabase(k int, dirName string) (*queryengine.QueryEngine, error) {
	bufferPool, err := storage.NewBufferPoolManager(k, dirName)
	if err != nil {
		return nil, fmt.Errorf("error initializing database: %w", err)
	}

	queryPtr := &queryengine.QueryEngine{
		DB: bufferPool,
	}

	go bufferPool.DiskScheduler.ProccessReq()
	log.Println("Database initialized successfully")
	return queryPtr, nil
}
