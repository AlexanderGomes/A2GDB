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
)

func main() {
	dm, _ := InitDatabase(replacerFrequency, dirName)

	dm.QueryEntryPoint(`CREATE TABLE Company (
			UserID INT AUTO_INCREMENT PRIMARY KEY,
			Username VARCHAR,
			PasswordHash VARCHAR
		);`)

	dm.QueryEntryPoint(`INSERT INTO Company (Username, PasswordHash) 
VALUES ('alex', '123');`)

	dm.QueryEntryPoint(`INSERT INTO Company (Username, PasswordHash) 
VALUES ('sander', '345');`)

	_, err := dm.QueryEntryPoint(`DELETE FROM Company WHERE Username = 'sander';`)
	if err != nil {
		fmt.Println(err, "first")
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
