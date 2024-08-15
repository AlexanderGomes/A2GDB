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
	t := storage.NewTree()
	for i := 1; i < 202; i++ {
		t.Insert(uint64(i), []byte(fmt.Sprintf("number: %d", i)))
	}

	leafMap := t.CreateLeafMap()
	bts, err := storage.EncodeBp(leafMap)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println(leafMap)

	fmt.Println(bts)
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
