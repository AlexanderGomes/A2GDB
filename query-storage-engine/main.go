package main

import (
	"a2gdb/cmd"
	"a2gdb/utils"
	"log"
)

func main() {
	engine, err := cmd.InitDatabase(2, "A2G_DB_OS")
	if err != nil {
		log.Fatal("DB init failed: ", err)
	}

	sql1 := "SELECT * FROM `User` \n"
	encodedPlan1, err := utils.SendSql(sql1)
	if err != nil {
		log.Panic(err)
	}

	_, _, res := engine.QueryProcessingEntry(encodedPlan1, false, false)
	if res.Error != nil {
		log.Fatal(res.Error)
	}
}

// ### SELECT * FROM `User`
// ### SELECT Username, Age FROM `User`
// Scan -> Project -> END

// ### SELECT * FROM `User` WHERE Email = 'sander@gmail.com'
// All queries containing a where clause at the end.
// Scan -> Filter -> Project -> END

// ### SELECT Username, Age, City FROM `User` ORDER BY Age ASC
// Scan -> Project -> Sort -> END

// ### SELECT City, COUNT(*) AS UserCount FROM `User` GROUP BY City
// Scan -> Project -> Aggregate -> END
