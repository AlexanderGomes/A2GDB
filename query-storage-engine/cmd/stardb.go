package cmd

import (
	"a2gdb/engines"
	"a2gdb/logger"
	"fmt"
)

func InitDatabase(k int, dirName string) (*engines.QueryEngine, error) {
	logger.InitLogger()
	bufferPool, err := engines.NewBufferPoolManager(k, dirName)
	if err != nil {
		return nil, fmt.Errorf("error initializing database: %w", err)
	}

	queryEngine := &engines.QueryEngine{
		BufferPoolManager: bufferPool,
	}

	logger.Log.Info("Database initialized successfully")
	return queryEngine, nil
}
