package main

import (
	queryengine "disk-db/query-engine"
	"disk-db/storage"
	"fmt"
	"log"
)

const (
	replacerFrequency = 2
	dirName           = "A2G_DB"
	NumPages          = (100 * 1024 * 1024 * 1024) / storage.PageSize
)

func main() {
	bp := storage.NewTree(10)

	for i := 0; i < 20000; i++ {
		bp.ReplaceOrInsert(storage.Item{Key: uint64(i), Value: []byte(fmt.Sprintf("number: %d", i))})
	}

	items := storage.GetAllItems(bp)
	fmt.Println(items)
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
