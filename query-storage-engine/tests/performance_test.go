package tests

import (
	"a2gdb/cmd"
	"a2gdb/engines"
	"fmt"
	"log"
	"testing"
)

func BenchmarkInsert(b *testing.B) {
	engineDB := InitDB("insert")

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sql1 := fmt.Sprintf("INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', %d, 'Los Angeles')\n", i+1)
		encodedPlan1, err := engines.SendSql(sql1)
		if err != nil {
			b.Fatal(err)
		}
		engineDB.QueryProcessingEntry(encodedPlan1, false, false)
	}
	b.StopTimer()
}

func BenchmarkDelete(b *testing.B) {
	engineDB := InitDB("delete")
	InsertSample(1000, engineDB)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sql1 := "DELETE FROM `User` WHERE Username = 'JaneSmith'\n"
		encodedPlan1, err := engines.SendSql(sql1)
		if err != nil {
			b.Fatal(err)
		}
		engineDB.QueryProcessingEntry(encodedPlan1, false, false)
	}
	b.StopTimer()
}

func BenchmarkUpdate(b *testing.B) {
	engineDB := InitDB("update")
	InsertSample(1000, engineDB)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sql1 := " UPDATE `User` SET Age = 121209 WHERE Username = 'JaneSmith'\n"
		encodedPlan1, err := engines.SendSql(sql1)
		if err != nil {
			b.Fatal(err)
		}
		engineDB.QueryProcessingEntry(encodedPlan1, false, false)
	}
	b.StopTimer()
}

func BenchmarkSelectWheres(b *testing.B) {
	engineDB := InitDB("wheres")
	InsertSample(1000, engineDB)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sql1 := "SELECT Username, Age, City FROM `User` WHERE Age > 20\n"
		encodedPlan1, err := engines.SendSql(sql1)
		if err != nil {
			b.Fatal(err)
		}
		engineDB.QueryProcessingEntry(encodedPlan1, false, false)
	}
	b.StopTimer()
}

func BenchmarkSelectWheresRange(b *testing.B) {
	engineDB := InitDB("wheresRange")
	InsertSample(1000, engineDB)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sql1 := "SELECT Username, Age, City FROM `User` WHERE Age BETWEEN 20 AND 30\n"
		encodedPlan1, err := engines.SendSql(sql1)
		if err != nil {
			b.Fatal(err)
		}
		engineDB.QueryProcessingEntry(encodedPlan1, false, false)
	}
	b.StopTimer()
}

func BenchmarkSelectSortingAsc(b *testing.B) {
	engineDB := InitDB("wheresSorting")
	InsertSample(1000, engineDB)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sql1 := "SELECT Username, Age, City FROM `User` ORDER BY Age ASC\n"
		encodedPlan1, err := engines.SendSql(sql1)
		if err != nil {
			b.Fatal(err)
		}
		engineDB.QueryProcessingEntry(encodedPlan1, false, false)
	}
	b.StopTimer()
}

func BenchmarkSelectSortingLimit(b *testing.B) {
	engineDB := InitDB("wheresSortingLimit")
	InsertSample(1000, engineDB)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sql1 := "SELECT Username, Age, City FROM `User` ORDER BY Age DESC LIMIT 1\n"
		encodedPlan1, err := engines.SendSql(sql1)
		if err != nil {
			b.Fatal(err)
		}
		engineDB.QueryProcessingEntry(encodedPlan1, false, false)
	}
	b.StopTimer()
}

func InsertSample(N int, engineDB *engines.QueryEngine) {
	for i := 0; i < N; i++ {
		sql1 := fmt.Sprintf("INSERT INTO `User` (Username, Age, City) VALUES ('JaneSmith', %d, 'Los Angeles')\n", i+1)
		encodedPlan1, err := engines.SendSql(sql1)
		if err != nil {
			log.Fatal(err)
		}
		engineDB.QueryProcessingEntry(encodedPlan1, false, false)
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
	encodedPlan1, err := engines.SendSql(sql)
	if err != nil {
		log.Fatal("Error getting query plan: ", err)
	}

	engineDB.QueryProcessingEntry(encodedPlan1, false, false)
}
