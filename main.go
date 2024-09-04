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

// 17227
func main() {
	dm, _ := InitDatabase(replacerFrequency, dirName)

	_, err := dm.QueryEntryPoint(`SELECT * FROM User WHERE Username = 'umtestingsansknaklsnklansklansklnaklsnkasn';`)
	if err != nil {
		fmt.Println(err)
	}
}

func Testing(dm *queryengine.QueryEngine) error {
	var err error

	_, err = dm.QueryEntryPoint(`CREATE TABLE User (
			UserID INT PRIMARY KEY,
			Username VARCHAR,
			Age INT,
			City VARCHAR
);`)

	for i := 0; i < 1000; i++ {
		_, err = dm.QueryEntryPoint(`INSERT INTO User (Username, Age, City) VALUES
		('sander1', 12, 'richmond'),
		('sander2', 15, 'richmond'),
		('sander3', 23, 'richmond'),
		('sander4', 11, 'richmond'),
		('sander5', 7, 'richmond');`)
		if err != nil {
			return err
		}

		_, err = dm.QueryEntryPoint(`INSERT INTO User (Username, Age, City) VALUES
		('sander6', 58, 'san pablo'),
		('sander7', 77, 'san pablo'),
		('sander8', 31, 'san pablo'),
		('sander9', 21, 'san pablo'),
		('sander10', 93, 'san pablo');`)
		if err != nil {
			return err
		}

		_, err = dm.QueryEntryPoint(`INSERT INTO User (Username, Age, City) VALUES
		('sander11', 16, 'pinole'),
		('sander12', 25, 'pinole'),
		('sander13', 11, 'pinole'),
		('sander14', 12, 'pinole'),
		('sander15', 10, 'pinole');`)
		if err != nil {
			return err
		}

		_, err = dm.QueryEntryPoint(`INSERT INTO User (Username, Age, City) VALUES
		('sander16', 90, 'san francisco'),
		('sander17', 97, 'san francisco'),
		('sander18', 93, 'san francisco'),
		('sander19', 95, 'san francisco'),
		('sander20', 91, 'san francisco');`)
		if err != nil {
			return err
		}
	}

	return err
}

func InitDatabase(k int, dirName string) (*queryengine.QueryEngine, error) {
	bufferPool, err := storage.NewBufferPoolManager(k, dirName)
	if err != nil {
		return nil, fmt.Errorf("error initializing database: %w", err)
	}

	queryPtr := &queryengine.QueryEngine{
		Disk: bufferPool.DiskManager,
	}

	log.Println("Database initialized successfully")
	return queryPtr, nil
}
