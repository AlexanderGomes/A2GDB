package main

import (
	"a2gdb/cmd"
	"a2gdb/engine"
	"fmt"
	"log"
)

func main() {
	engine, err := cmd.InitDatabase(2, "A2G_DB")
	if err != nil {
		log.Fatal("DB init failed: ", err)
	}

	sql := "UPDATE `User` SET Age = 82828282 WHERE UserId = CAST('13045412650426019748' AS DECIMAL(20,0))\n"
	selects(engine, sql)
}

func selects(engineM *engine.QueryEngine, sql string) {
	encodedPlan1, err := engine.SendSql(sql)
	if err != nil {
		log.Fatal(err)
	}

	_, _, result := engineM.EngineEntry(encodedPlan1, false, true)
	if result.Error != nil {
		log.Fatal(result.Error)
	}
}

func insertMany(engineM *engine.QueryEngine) {
	for i := range 100 {
		random := engine.GenerateRandomNumber()
		sql1 := fmt.Sprintf("INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', %d, 'Los Angeles')\n", i+random)

		encodedPlan1, err := engine.SendSql(sql1)
		if err != nil {
			log.Fatal(err)
		}
		_, _, result := engineM.EngineEntry(encodedPlan1, false, false)
		if result.Error != nil {
			fmt.Println(result.Error)
		}
	}
}

func createTable(engineM *engine.QueryEngine) {
	sql := "CREATE TABLE `User`(PRIMARY KEY(UserId), Username VARCHAR, Age INT, City VARCHAR) \n"

	encodedPlan1, err := engine.SendSql(sql)
	if err != nil {
		log.Fatal(err)
	}
	_, _, result := engineM.EngineEntry(encodedPlan1, false, false)

	if result.Error != nil {
		log.Fatal("couldn't create page, error: ", result.Error)
	}
}
