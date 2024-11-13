package cmd

import (
	"disk-db/logger"
	queryengine "disk-db/query-engine"
	"disk-db/storage-engine"
	"fmt"
)

func InitDatabase(k int, dirName string) (*queryengine.QueryEngine, error) {
	logger.InitLogger()
	bufferPool, err := storage.NewBufferPoolManager(k, dirName)
	if err != nil {
		return nil, fmt.Errorf("error initializing database: %w", err)
	}

	queryPtr := &queryengine.QueryEngine{
		Disk: bufferPool.DiskManager,
	}

	logger.Log.Info("Database initialized successfully")
	return queryPtr, nil
}
