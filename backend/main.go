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

	createTable(engine)
	//insertMany(engine)
	selects(engine)
}

func selects(engine *engine.QueryEngine) {
	sql1 := "SELECT User.Username, User.Age, User.City, Orders.OrderAmount FROM `User` JOIN Orders ON User.UserId = Orders.UserId\n"
	encodedPlan1, err := util.SendSql(sql1)
	if err != nil {
		log.Fatal(err)
	}
	// engine.EngineEntry(encodedPlan1)
	fmt.Println(encodedPlan1)
}

func insertMany(engine *engine.QueryEngine) {
	for i := range 1000 {
		sql1 := "INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', 25, 'Los Angeles')\n"
		sql2 := fmt.Sprintf("INSERT INTO `Orders` (Username, OrderAmount) VALUES ('JaneSmith', %d)\n", i+122)

		encodedPlan1, err := util.SendSql(sql1)
		if err != nil {
			log.Fatal(err)
		}
		engine.EngineEntry(encodedPlan1)

		encodedPlan2, err := util.SendSql(sql2)
		if err != nil {
			log.Fatal(err)
		}
		engine.EngineEntry(encodedPlan2)
	}
}

func createTable(engine *engine.QueryEngine) {
	sql := "CREATE TABLE `User`(PRIMARY KEY(UserId), Username VARCHAR, Age INT, City VARCHAR) \n"
	sql2 := "CREATE TABLE `Orders`(PRIMARY KEY(OrderId), OrderAmount INT, PRIMARY KEY (UserId))  \n"

	encodedPlan1, err := util.SendSql(sql)
	if err != nil {
		log.Fatal(err)
	}
	engine.EngineEntry(encodedPlan1)

	encodedPlan2, err := util.SendSql(sql2)
	if err != nil {
		log.Fatal(err)
	}
	engine.EngineEntry(encodedPlan2)
}
