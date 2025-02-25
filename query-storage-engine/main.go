package main

import (
	"a2gdb/utils"
	"fmt"
	"log"
)

func main() {
	// engine, err := cmd.InitDatabase(2, "A2G_DB_OS")
	// if err != nil {
	// 	log.Fatal("DB init failed: ", err)
	// }

	sql1 := "SELECT City, COUNT(*) AS UserCount FROM `User` GROUP BY City\n"
	encodedPlan1, err := utils.SendSql(sql1)
	if err != nil {
		log.Panic(err)
	}

	fmt.Println(encodedPlan1)
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