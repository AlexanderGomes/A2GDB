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
	dm, _ := InitDatabase(replacerFrequency, dirName)

	dm.QueryEntryPoint(`CREATE TABLE User (
			UserID INT PRIMARY KEY,
			Username VARCHAR,
			PasswordHash VARCHAR
		);`)

	for i := 0; i < 1000; i++ {
		_, err := dm.QueryEntryPoint(`INSERT INTO User (UserID, Username, PasswordHash) VALUES
			(31, 'john_doe', 'hashed_password_1'),
			(31, 'john_doe', 'hashed_password_1'),
			(31, 'john_doe', 'hashed_password_1'),
			(31, 'john_doe', 'hashed_password_1'),
			(31, 'john_doe', 'hashed_password_1'),
			(31, 'john_doe', 'hashed_password_1');`)

		if err != nil {
			fmt.Println(err)
		}
	}
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
