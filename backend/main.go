package main

import (
	"a2gdb/cmd"
	"a2gdb/util"
	"log"
)

func main() {
	engine, err := cmd.InitDatabase(2, "A2G_DB")
	if err != nil {
		log.Fatal("DB init failed: ", err)
	}

	sql1 := "INSERT INTO `User` (Username, Age, City) VALUES ('sander0909', 18, 'Richmond'), ('john_doe', 25, 'New York'), ('jane_smith', 30, 'Los Angeles')\n"
	encodedPlan1 := util.SendSql(sql1)

	engine.EngineEntry(encodedPlan1)

}
