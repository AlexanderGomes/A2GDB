package cmd

import (
	"a2gdb/engines"
	"a2gdb/logger"
	"a2gdb/utils"
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

	if err := CreateDefaultTable(queryEngine); err != nil {
		return nil, fmt.Errorf("CreateDefaultTable failed: %w", err)
	}

	logger.Log.Info("Database initialized successfully")
	return queryEngine, nil
}

func CreateDefaultTable(queryEngine *engines.QueryEngine) error {
	sql := "CREATE TABLE `User`(PRIMARY KEY(UserId),Email VARCHAR, Password VARCHAR, DbPath VARCHAR)\n"
	encodedPlan1, err := utils.SendSql(sql)
	if err != nil {
		return fmt.Errorf("SendSql failed: %w", err)
	}

	_, _, result := queryEngine.QueryProcessingEntry(encodedPlan1, false, false)

	if result.Error != nil {
		return fmt.Errorf("QueryProcessingEntry failed: %w", err)
	}

	return nil
}
