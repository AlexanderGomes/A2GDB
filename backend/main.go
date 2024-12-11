package main

import (
	"a2gdb/cmd"
	"a2gdb/query-engine/engine"
	"a2gdb/util"
	"fmt"
	"log"
)

func main() {
	engine, err := cmd.InitDatabase(2, "A2G_DB")
	if err != nil {
		log.Fatal("DB init failed: ", err)
	}

	selects(engine)
}

func selects(engine *engine.QueryEngine) {
	sql1 := "SELECT City, MAX(Age) AS oldest_in_city FROM `User` GROUP BY City\n"
	encodedPlan1 := util.SendSql(sql1)
	fmt.Println(encodedPlan1)
	//engine.EngineEntry(encodedPlan1)
}

func insertMany(engine *engine.QueryEngine) {
	for range 5000 {
		sql1 := "INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', 25, 'Los Angeles'), ('AliceBrown', 28, 'Chicago'), ('BobWhite', 35, 'Houston') \n"
		encodedPlan1 := util.SendSql(sql1)
		engine.EngineEntry(encodedPlan1)
	}
}

func createTable(engine *engine.QueryEngine) {
	sql := "CREATE TABLE `User`(PRIMARY KEY(UserId), Username VARCHAR, Age INT, City VARCHAR)\n"
	encodedPlan1 := util.SendSql(sql)
	engine.EngineEntry(encodedPlan1)
}
