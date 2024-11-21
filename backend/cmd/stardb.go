package cmd

import (
	"a2gdb/query-engine/engine"
	"a2gdb/storage-engine/logger"
	"a2gdb/storage-engine/storage"
	"fmt"
)

func InitDatabase(k int, dirName string) (*engine.QueryEngine, error) {
	logger.InitLogger()
	bufferPool, err := storage.NewBufferPoolManager(k, dirName)
	if err != nil {
		return nil, fmt.Errorf("error initializing database: %w", err)
	}

	queryEngine := &engine.QueryEngine{
		StorageManager: bufferPool.DiskManager,
	}

	logger.Log.Info("Database initialized successfully")
	return queryEngine, nil
}
