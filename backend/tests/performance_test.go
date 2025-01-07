package tests

import (
	"a2gdb/cmd"
	"a2gdb/query-engine/engine"
	"a2gdb/util"
	"fmt"
	"log"
	"testing"
)

func BenchmarkInsert(b *testing.B) {
	engine := InitDB("insert")

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sql1 := fmt.Sprintf("INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', %d, 'Los Angeles')\n", i+1)
		encodedPlan1, err := util.SendSql(sql1)
		if err != nil {
			b.Fatal(err)
		}
		engine.EngineEntry(encodedPlan1)
	}
	b.StopTimer()
}

func BenchmarkDelete(b *testing.B) {
	engine := InitDB("delete")
	InsertSample(1000, engine)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sql1 := "DELETE FROM `User` WHERE Username = 'JaneSmith'\n"
		encodedPlan1, err := util.SendSql(sql1)
		if err != nil {
			b.Fatal(err)
		}
		engine.EngineEntry(encodedPlan1)
	}
	b.StopTimer()
}

func BenchmarkUpdate(b *testing.B) {
	engine := InitDB("update")
	InsertSample(1000, engine)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sql1 := " UPDATE `User` SET Age = 121209 WHERE Username = 'JaneSmith'\n"
		encodedPlan1, err := util.SendSql(sql1)
		if err != nil {
			b.Fatal(err)
		}
		engine.EngineEntry(encodedPlan1)
	}
	b.StopTimer()
}

func BenchmarkSelectWheres(b *testing.B) {
	engine := InitDB("wheres")
	InsertSample(1000, engine)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sql1 := "SELECT Username, Age, City FROM `User` WHERE Age > 20\n"
		encodedPlan1, err := util.SendSql(sql1)
		if err != nil {
			b.Fatal(err)
		}
		engine.EngineEntry(encodedPlan1)
	}
	b.StopTimer()
}

func BenchmarkSelectWheresRange(b *testing.B) {
	engine := InitDB("wheresRange")
	InsertSample(1000, engine)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sql1 := "SELECT Username, Age, City FROM `User` WHERE Age BETWEEN 20 AND 30\n"
		encodedPlan1, err := util.SendSql(sql1)
		if err != nil {
			b.Fatal(err)
		}
		engine.EngineEntry(encodedPlan1)
	}
	b.StopTimer()
}

func BenchmarkSelectSortingAsc(b *testing.B) {
	engine := InitDB("wheresSorting")
	InsertSample(1000, engine)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sql1 := "SELECT Username, Age, City FROM `User` ORDER BY Age ASC\n"
		encodedPlan1, err := util.SendSql(sql1)
		if err != nil {
			b.Fatal(err)
		}
		engine.EngineEntry(encodedPlan1)
	}
	b.StopTimer()
}

func BenchmarkSelectSortingLimit(b *testing.B) {
	engine := InitDB("wheresSortingLimit")
	InsertSample(1000, engine)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sql1 := "SELECT Username, Age, City FROM `User` ORDER BY Age DESC LIMIT 1\n"
		encodedPlan1, err := util.SendSql(sql1)
		if err != nil {
			b.Fatal(err)
		}
		engine.EngineEntry(encodedPlan1)
	}
	b.StopTimer()
}

func InsertSample(N int, engine *engine.QueryEngine) {
	for i := 0; i < N; i++ {
		sql1 := fmt.Sprintf("INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', %d, 'Los Angeles')\n", i+1)
		encodedPlan1, err := util.SendSql(sql1)
		if err != nil {
			log.Fatal(err)
		}
		engine.EngineEntry(encodedPlan1)
	}
}

func InitDB(testName string) *engine.QueryEngine {
	engine, err := cmd.InitDatabase(2, fmt.Sprintf("./%s", testName))
	if err != nil {
		log.Fatalf("Initializing DB failed: %s", err)
	}

	CreateTable(engine)
	return engine
}

func CreateTable(e *engine.QueryEngine) {
	sql := "CREATE TABLE `User`(PRIMARY KEY(UserId), Username VARCHAR, Age INT, City VARCHAR)\n"
	encodedPlan1, err := util.SendSql(sql)
	if err != nil {
		log.Fatal("Error getting query plan: ", err)
	}

	e.EngineEntry(encodedPlan1)
}
