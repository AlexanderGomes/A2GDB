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
	sql1 := "SELECT Username, Age, City FROM `User` WHERE Age BETWEEN 30 AND 40\n"
	encodedPlan1 := util.SendSql(sql1)
	engine.EngineEntry(encodedPlan1)
}

func insertMany(engine *engine.QueryEngine) {
	for i := range 5000 {
		sql1 := fmt.Sprintf("INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', %d, 'Los Angeles')\n", i)
		sql2 := fmt.Sprintf("INSERT INTO `User` (Username, Age, City) VALUES ('AliceBrown', %d, 'Chicago')\n", i+5)
		sql3 := fmt.Sprintf("INSERT INTO `User` (Username, Age, City) VALUES ('BobWhite', %d, 'Houston')\n", i+20)

		encodedPlan1 := util.SendSql(sql1)
		engine.EngineEntry(encodedPlan1)

		encodedPlan2 := util.SendSql(sql2)
		engine.EngineEntry(encodedPlan2)

		encodedPlan3 := util.SendSql(sql3)
		engine.EngineEntry(encodedPlan3)
	}
}

func createTable(engine *engine.QueryEngine) {
	sql := "CREATE TABLE `User`(PRIMARY KEY(UserId), Username VARCHAR, Age INT, City VARCHAR)\n"
	encodedPlan1 := util.SendSql(sql)
	engine.EngineEntry(encodedPlan1)
}
