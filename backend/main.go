package main

import (
	"a2gdb/cmd"
	"a2gdb/query-engine/engine"
	"a2gdb/util"

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
	sql1 := "UPDATE `User` SET Age = 121276 WHERE Username = 'JaneSmith'\n"
	encodedPlan1, err := util.SendSql(sql1)
	if err != nil {
		log.Fatal(err)
	}

	_, _, result := engine.EngineEntry(encodedPlan1)
	if result.Error != nil {
		log.Println(result.Error)
	}

	log.Println(result.Rows)
}

func insertMany(engine *engine.QueryEngine) {
	for range 1000 {
		sql1 := "INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', 25, 'Los Angeles')\n"

		encodedPlan1, err := util.SendSql(sql1)
		if err != nil {
			log.Fatal(err)
		}
		_, _, result := engine.EngineEntry(encodedPlan1)
		if result.Error != nil {
			log.Println(result.Error)
		}

		log.Println(result.Msg)
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
		log.Println("couldn't create page, error: ", result.Error)
		return
	}

	log.Println(result.Msg)
}
