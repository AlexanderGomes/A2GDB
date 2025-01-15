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
	sql1 := "UPDATE `User` SET Age =  10101010  WHERE Username = 'JaneSmith'\n"
	encodedPlan1, err := util.SendSql(sql1)
	if err != nil {
		log.Fatal(err)
	}

	_, _, result := engine.EngineEntry(encodedPlan1)
	if result.Error != nil {
		log.Fatal(result.Error)
	}
}

func insertMany(engine *engine.QueryEngine) {
	for i := range 10000 {
		random := util.GenerateRandomNumber()
		sql1 := fmt.Sprintf("INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', %d, 'Los Angeles')\n", i+random)

		encodedPlan1, err := util.SendSql(sql1)
		if err != nil {
			log.Fatal(err)
		}
		_, _, result := engine.EngineEntry(encodedPlan1)
		if result.Error != nil {
			log.Fatal(result.Error)
		}
	}
}

func createTable(engine *engine.QueryEngine) {
	sql := "CREATE TABLE `User`(PRIMARY KEY(UserId), Username VARCHAR, Age INT, City VARCHAR) \n"

	encodedPlan1, err := util.SendSql(sql)
	if err != nil {
		log.Fatal(err)
	}
	_, _, result := engine.EngineEntry(encodedPlan1)

	if result.Error != nil {
		log.Fatal("couldn't create page, error: ", result.Error)
	}
}
