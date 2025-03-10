package tests

import (
	"a2gdb/cmd"
	"a2gdb/engines"
	"a2gdb/utils"
	"fmt"
	"log"
	"testing"
)

func BenchmarkInsert(b *testing.B) {
	engineDB := InitDB("insert")

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sql1 := fmt.Sprintf("INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', %d, 'Los Angeles')\n", i+1)
		encodedPlan, err := utils.SendSql(sql1)
		if err != nil {
			b.Fatal(err)
		}
		queryInfo := engines.QueryInfo{Id: engines.GenerateRandomID(), RawPlan: encodedPlan}
		engineDB.QueryProcessingEntry(&queryInfo)
	}
	b.StopTimer()
}

func BenchmarkDelete(b *testing.B) {
	engineDB := InitDB("delete")
	InsertSample(1000, engineDB)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sql1 := "DELETE FROM `User` WHERE Username = 'JaneSmith'\n"
		encodedPlan, err := utils.SendSql(sql1)
		if err != nil {
			b.Fatal(err)
		}
		queryInfo := engines.QueryInfo{Id: engines.GenerateRandomID(), RawPlan: encodedPlan}
		engineDB.QueryProcessingEntry(&queryInfo)
	}
	b.StopTimer()
}

func BenchmarkUpdate(b *testing.B) {
	engineDB := InitDB("update")
	InsertSample(1000, engineDB)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sql1 := " UPDATE `User` SET Age = 121209 WHERE Username = 'JaneSmith'\n"
		encodedPlan, err := utils.SendSql(sql1)
		if err != nil {
			b.Fatal(err)
		}
		queryInfo := engines.QueryInfo{Id: engines.GenerateRandomID(), RawPlan: encodedPlan}
		engineDB.QueryProcessingEntry(&queryInfo)
	}
	b.StopTimer()
}

func BenchmarkSelectWheres(b *testing.B) {
	engineDB := InitDB("wheres")
	InsertSample(1000, engineDB)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sql1 := "SELECT Username, Age, City FROM `User` WHERE Age > 20\n"
		encodedPlan, err := utils.SendSql(sql1)
		if err != nil {
			b.Fatal(err)
		}
		queryInfo := engines.QueryInfo{Id: engines.GenerateRandomID(), RawPlan: encodedPlan}
		engineDB.QueryProcessingEntry(&queryInfo)
	}
	b.StopTimer()
}

func BenchmarkSelectWheresRange(b *testing.B) {
	engineDB := InitDB("wheresRange")
	InsertSample(1000, engineDB)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sql1 := "SELECT Username, Age, City FROM `User` WHERE Age BETWEEN 20 AND 30\n"
		encodedPlan, err := utils.SendSql(sql1)
		if err != nil {
			b.Fatal(err)
		}
		queryInfo := engines.QueryInfo{Id: engines.GenerateRandomID(), RawPlan: encodedPlan}
		engineDB.QueryProcessingEntry(&queryInfo)
	}
	b.StopTimer()
}

func BenchmarkSelectSortingAsc(b *testing.B) {
	engineDB := InitDB("wheresSorting")
	InsertSample(1000, engineDB)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sql1 := "SELECT Username, Age, City FROM `User` ORDER BY Age ASC\n"
		encodedPlan, err := utils.SendSql(sql1)
		if err != nil {
			b.Fatal(err)
		}
		queryInfo := engines.QueryInfo{Id: engines.GenerateRandomID(), RawPlan: encodedPlan}
		engineDB.QueryProcessingEntry(&queryInfo)
	}
	b.StopTimer()
}

func BenchmarkSelectSortingLimit(b *testing.B) {
	engineDB := InitDB("wheresSortingLimit")
	InsertSample(1000, engineDB)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sql1 := "SELECT Username, Age, City FROM `User` ORDER BY Age DESC LIMIT 1\n"
		encodedPlan, err := utils.SendSql(sql1)
		if err != nil {
			b.Fatal(err)
		}
		queryInfo := engines.QueryInfo{Id: engines.GenerateRandomID(), RawPlan: encodedPlan}
		engineDB.QueryProcessingEntry(&queryInfo)
	}
	b.StopTimer()
}

func InsertSample(N int, engineDB *engines.QueryEngine) {
	for i := 0; i < N; i++ {
		sql1 := fmt.Sprintf("INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', %d, 'Los Angeles')\n", i+1)
		encodedPlan, err := utils.SendSql(sql1)
		if err != nil {
			log.Fatal(err)
		}
		queryInfo := engines.QueryInfo{Id: engines.GenerateRandomID(), RawPlan: encodedPlan}
		engineDB.QueryProcessingEntry(&queryInfo)
	}
}

func InitDB(testName string) *engines.QueryEngine {
	engines, err := cmd.InitDatabase(2, fmt.Sprintf("./%s", testName))
	if err != nil {
		log.Fatalf("Initializing DB failed: %s", err)
	}

	CreateTable(engines)
	return engines
}

func CreateTable(engineDB *engines.QueryEngine) {
	sql := "CREATE TABLE `User`(PRIMARY KEY(UserId), Username VARCHAR, Age INT, City VARCHAR)\n"
	encodedPlan, err := utils.SendSql(sql)
	if err != nil {
		log.Fatal("Error getting query plan: ", err)
	}

	queryInfo := engines.QueryInfo{Id: engines.GenerateRandomID(), RawPlan: encodedPlan}
	engineDB.QueryProcessingEntry(&queryInfo)
}
