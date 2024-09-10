package cmd

import (
	"disk-db/query-engine"
	"disk-db/storage"
	"fmt"
	"log"
)

func InitDatabase(k int, dirName string) (*queryengine.QueryEngine, error) {
	bufferPool, err := storage.NewBufferPoolManager(k, dirName)
	if err != nil {
		return nil, fmt.Errorf("error initializing database: %w", err)
	}

	queryPtr := &queryengine.QueryEngine{
		Disk: bufferPool.DiskManager,
	}

	log.Println("Database initialized successfully")
	return queryPtr, nil
}
