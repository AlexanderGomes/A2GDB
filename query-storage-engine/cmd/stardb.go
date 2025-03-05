package cmd

import (
	"a2gdb/engines"
	"a2gdb/logger"
	"a2gdb/utils"
	"fmt"
	"strings"
	"sync"
)

func InitDatabase(k int, dirName string) (*engines.QueryEngine, error) {
	logger.InitLogger()
	bufferPool, err := engines.NewBufferPoolManager(k, dirName)
	if err != nil {
		return nil, fmt.Errorf("error initializing database: %w", err)
	}

	queryEngine := &engines.QueryEngine{
		BufferPoolManager: bufferPool,
		Lm:                &engines.LockManager{Mu: sync.RWMutex{}, Rows: map[uint64]*engines.RowInfo{}},
		QueryChan:         make(chan *engines.QueryInfo, 10000),
		ResChan:           make(chan *engines.Result, 10000),
	}

	if err := CreateDefaultTable(queryEngine); err != nil {
		if !strings.Contains(err.Error(), "table already exists") {
			return nil, fmt.Errorf("unexpected Error: %w", err)
		}
	}

	go queryEngine.QueryManager()

	logger.Log.Info("Database initialized successfully")
	return queryEngine, nil
}

func CreateDefaultTable(queryEngine *engines.QueryEngine) error {
	sql := "CREATE TABLE `User`(PRIMARY KEY(UserId), Username VARCHAR, Age INT, City VARCHAR) \n"
	encodedPlan1, err := utils.SendSql(sql)
	if err != nil {
		return fmt.Errorf("SendSql failed: %w", err)
	}

	result := queryEngine.QueryProcessingEntry(encodedPlan1, false, false)
	if result.Error != nil {
		return fmt.Errorf("QueryProcessingEntry failed: %w", result.Error)
	}

	return nil
}
