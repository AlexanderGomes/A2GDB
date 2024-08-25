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
	NumPages          = (100 * 1024 * 1024 * 1024) / storage.PageSizeV2
)

func main() {
	dm, _ := InitDatabase(replacerFrequency, dirName)

	res, err := dm.QueryEntryPoint(`UPDATE User SET Username = '1912992992929292991929192919291929192' WHERE UserID = 15476753473141262555;`)

	if err != nil {
		fmt.Println(err)
	}

	fmt.Println(res)
}

func Testing(dm *queryengine.QueryEngine) (queryengine.Query, error) {
	dm.QueryEntryPoint(`CREATE TABLE User (
		UserID INT PRIMARY KEY,
		Username VARCHAR,
		PasswordHash VARCHAR
);`)

	for i := 0; i < 2000; i++ {
		dm.QueryEntryPoint(`INSERT INTO User (Username, PasswordHash) VALUES
		('sander', 'hashed_password_1'),
		('john_doe', 'hashed_password_1'),
		('john_doe', 'hashed_password_1'),
		('john_doe', 'hashed_password_1'),
		('john_doe', 'hashed_password_1');`)
	}

	res, err := dm.QueryEntryPoint(`SELECT * FROM User WHERE Username = 'sander';`)
	if err != nil {
		return queryengine.Query{}, err
	}

	return res, nil
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
