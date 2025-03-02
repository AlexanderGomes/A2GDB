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

	queries := []string{
		"SELECT Username, Age, City FROM `User` ORDER BY Age DESC LIMIT 1",
		"DELETE FROM `User` WHERE Username = 'JaneSmith'",
		"SELECT Username, Age, City FROM `User` WHERE Age > 2",
		"UPDATE `User` SET Age = 121209 WHERE Username = 'AliceBrown'",
		"SELECT * FROM `User`",
	}

	var wg sync.WaitGroup
	for i := range 40 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			query := queries[i%len(queries)]
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

	res := engine.QueryProcessingEntry(encodedPlan1, false, false)
	if res.Error != nil {
		log.Fatal(res.Error)
	}

	fmt.Println(len(res.Rows))
}
