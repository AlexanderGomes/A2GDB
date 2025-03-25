package main

import (
	"a2gdb/cmd"
	"a2gdb/engines"
	"a2gdb/utils"
	"fmt"
	"log"
	"sync"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load("../.env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	engine, err := cmd.InitDatabase(2, "A2G_DB_OS")
	if err != nil {
		log.Fatal("DB init failed: ", err)
	}

	server := engines.NewServer(&engines.Config{Host: "localhost", Port: "3030", QueryEngine: engine})
	server.Run()
}

func InsertMany(engine *engines.QueryEngine, x int) {
	sql := "INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', 25, 'Los Angeles'), ('AliceBrown', 28, 'Chicago'), ('BobWhite', 35, 'Houston')"
	for range x {
		sendQuery(engine, sql)
	}
}

func Concurrent(engine *engines.QueryEngine, queries []string) {
	var wg sync.WaitGroup
	for i := range len(queries) {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			query := queries[i]
			sendQuery(engine, query)
		}(i)
	}

	wg.Wait()
}

func sendQuery(engine *engines.QueryEngine, sql string) {
	encodedPlan1, err := utils.SendSql(sql)
	if err != nil {
		log.Panic(err)
	}

	queryInfo := engines.QueryInfo{Id: engines.GenerateRandomID(), RawPlan: encodedPlan1, TransactionOff: false, InduceErr: false}

	resChan := engine.ResultManager.CreatePersonalChan()
	engine.ResultManager.Subscribe(queryInfo.Id, resChan)

	engine.QueryChan <- &queryInfo
	res := <-resChan

	if res.Error != nil {
		panic(res.Error)
	}
	fmt.Printf("query: %s, len: %d\n", sql, len(res.Rows))
}
