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
	dm, err := InitDatabase(replacerFrequency, dirName)

	if err != nil {
		fmt.Println("qryEngine: %w", err)
	}

	dm.QueryEntryPoint(`CREATE TABLE Company (
		UserID INT AUTO_INCREMENT PRIMARY KEY,
		Username VARCHAR,
		PasswordHash VARCHAR
	);`)

	for i := 0; i < 1000; i++ {
		_, err := dm.QueryEntryPoint(`INSERT INTO Company (Username, PasswordHash) 
	VALUES ('sander', '123');`)

		dm.QueryEntryPoint(`INSERT INTO Company (Username, PasswordHash) 
	VALUES ('alex', '456');`)

		dm.QueryEntryPoint(`INSERT INTO Company (Username, PasswordHash) 
	VALUES ('malu', '789');`)

		if err != nil {
			fmt.Println("qryEngine: %w", err)
		}
	}

	res, err := dm.QueryEntryPoint(`SELECT Username, PasswordHash FROM Company WHERE Username = 'sander';`)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println(res.Result)
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
