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

	schedulerNotification := make(chan *engines.Result, 1000)
	globalChannel := make(chan *engines.Result, 1000)
	queryEngine := &engines.QueryEngine{
		BufferPoolManager: bufferPool,
		Lm:                &engines.LockManager{Mu: sync.RWMutex{}, Rows: map[uint64]*engines.RowInfo{}},
		QueryChan:         make(chan *engines.QueryInfo, 1000),
		ResultManager:     &engines.ResultManager{SubscribedQueries: map[uint64]chan *engines.Result{}, GlobalChannel: globalChannel, SchedulerNotification: schedulerNotification},
	}

	queryEngine.Scheduler = engines.NewQueryScheduler(schedulerNotification, globalChannel, queryEngine)

	if err := CreateDefaultTable(queryEngine); err != nil {
		if !strings.Contains(err.Error(), "table already exists") {
			return nil, fmt.Errorf("unexpected Error: %w", err)
		}
	}

	go queryEngine.QueryManager()
	go queryEngine.ResultManager.ResultCollector()
	go queryEngine.Scheduler.Scheduler()
	go queryEngine.Scheduler.Decreaser()

	logger.Log.Info("Database initialized successfully")
	return queryEngine, nil
}

func CreateDefaultTable(queryEngine *engines.QueryEngine) error {
	sql := "CREATE TABLE `User`(PRIMARY KEY(UserId), Username VARCHAR, Age INT, City VARCHAR) \n"
	encodedPlan1, err := utils.SendSql(sql)
	if err != nil {
		return fmt.Errorf("SendSql failed: %w", err)
	}

	queryInfo := engines.QueryInfo{RawPlan: encodedPlan1, TransactionOff: false, InduceErr: false, Id: engines.GenerateRandomID()}
	result := queryEngine.QueryProcessingEntry(&queryInfo)
	if result.Error != nil {
		return fmt.Errorf("QueryProcessingEntry failed: %w", result.Error)
	}

	return nil
}
