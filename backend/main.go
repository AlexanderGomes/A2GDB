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

	insertMany(engine)
}

func selects(engine *engine.QueryEngine) {
	sql1 := "DELETE FROM `User` WHERE Username = 'BobWhite'\n"
	encodedPlan1 := util.SendSql(sql1)
	engine.EngineEntry(encodedPlan1)
}

func insertMany(engine *engine.QueryEngine) {
	for i := range 10000 {
		sql1 := fmt.Sprintf("INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', %d, 'Los Angeles')\n", i)


		encodedPlan1 := util.SendSql(sql1)
		engine.EngineEntry(encodedPlan1)
	}
}

func createTable(engine *engine.QueryEngine) {
	sql := "CREATE TABLE `User`(PRIMARY KEY(UserId), Username VARCHAR, Age INT, City VARCHAR)\n"
	encodedPlan1 := util.SendSql(sql)
	engine.EngineEntry(encodedPlan1)
}
