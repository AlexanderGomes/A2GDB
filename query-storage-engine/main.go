package main

import (
	"a2gdb/cmd"
	"a2gdb/engines"
	"a2gdb/utils"
	"fmt"
	"log"
	"sync"
)

func main() {
	engine, err := cmd.InitDatabase(2, "A2G_DB_OS")
	if err != nil {
		log.Fatal("DB init failed: ", err)
	}

	// InsertMany(engine, 2000)

	queries := []string{
		"SELECT * FROM `User`",
		"SELECT Username, Age FROM `User`",
		"INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', 25, 'Los Angeles'), ('AliceBrown', 28, 'Chicago'), ('BobWhite', 35, 'Houston')",
		"SELECT Username, Age, City FROM `User` WHERE Age > 20",
		"SELECT Username, Age, City FROM `User` WHERE Age = 20",
		"INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', 25, 'Los Angeles'), ('AliceBrown', 28, 'Chicago'), ('BobWhite', 35, 'Houston')",
		"SELECT Username, Age, City FROM `User` WHERE Age < 20",
		"INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', 25, 'Los Angeles'), ('AliceBrown', 28, 'Chicago'), ('BobWhite', 35, 'Houston')",
		"SELECT * FROM `User`",
		"SELECT Username, Age FROM `User`",
		"INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', 25, 'Los Angeles'), ('AliceBrown', 28, 'Chicago'), ('BobWhite', 35, 'Houston')",
		"SELECT Username, Age, City FROM `User` WHERE Age > 20",
		"SELECT Username, Age, City FROM `User` WHERE Age = 20",
		"INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', 25, 'Los Angeles'), ('AliceBrown', 28, 'Chicago'), ('BobWhite', 35, 'Houston')",
		"SELECT Username, Age, City FROM `User` WHERE Age < 20",
		"INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', 25, 'Los Angeles'), ('AliceBrown', 28, 'Chicago'), ('BobWhite', 35, 'Houston')",
	}

	Concurrent(engine, queries)
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
