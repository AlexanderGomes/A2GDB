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
	for i := 0; i < 2000; i++ {
		t.Insert(uint64(i), []byte("name inserted"))
	}

	visited := make(map[*storage.Node]bool)
	bts, err := storage.EncodeNode(t.Root, visited)
	if err != nil {
		fmt.Println(err)
	}

	node, err := storage.DecodeNode(bts)

	if err != nil {
		fmt.Println(err)
	}

	fmt.Println(node)
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
