package cmd

import (
	"a2gdb/engine"
	"a2gdb/logger"
	"fmt"
)

func InitDatabase(k int, dirName string) (*engine.QueryEngine, error) {
	logger.InitLogger()
	bufferPool, err := engine.NewBufferPoolManager(k, dirName)
	if err != nil {
		return nil, fmt.Errorf("error initializing database: %w", err)
	}

	queryEngine := &engine.QueryEngine{
		BufferPoolManager: bufferPool,
	}

	logger.Log.Info("Database initialized successfully")
	return queryEngine, nil
}
