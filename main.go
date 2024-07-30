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
	manager, err := storage.NewDiskManagerV2(dirName)

	if err != nil {
		fmt.Print(err)
	}

	manager.CreateTable("company", storage.TableInfo{})
	manager.CreateTable("house", storage.TableInfo{})
	manager.CreateTable("casa", storage.TableInfo{})

	manager.InMemoryTableSetUp("company")
	manager.InMemoryTableSetUp("casa")
	manager.InMemoryTableSetUp("house")

	page := storage.Page{
		ID:    929192991929,
		TABLE: "casa",
	}

	diskreq := storage.DiskReq{
		Page: page,
	}

	err = manager.WriteToDisk(diskreq)

	fmt.Println(err)
	manager.WriteCatalog()
}

func InitDatabase(k int, fileName string) (*queryengine.QueryEngine, error) {
	bufferPool, err := storage.NewBufferPoolManager(k, fileName)
	if err != nil {
		log.Println("Error initializing database:")
		return nil, err
	}

	queryPtr := &queryengine.QueryEngine{
		DB: bufferPool,
	}

	go bufferPool.DiskScheduler.ProccessReq()
	log.Println("Database initialized successfully")
	return queryPtr, nil
}
