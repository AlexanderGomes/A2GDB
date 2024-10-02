package main

import (
	"disk-db/cmd"
	queryengine "disk-db/query-engine"
	"log"
)

const (
	replacerFrequency = 2
	dirName           = "A2G_DB"
)

func main() {
	dm, _ := cmd.InitDatabase(replacerFrequency, dirName)
	if err := Testing(dm); err != nil {
		log.Fatal(err)
	}
}

func Testing(dm *queryengine.QueryEngine) error {
	var err error

	// for i := 0; i < 5000; i++ {
	// 	_, err = dm.QueryEntryPoint(`INSERT INTO User (Username, Age, City) VALUES
	// 	('sander1', 12, 'san pablo'),
	// 	('sander2', 15, 'san pablo'),
	// 	('sander3', 23, 'richmond'),
	// 	('sander4', 11, 'hercules'),
	// 	('sander5', 7, 'hercules');`)
	// 	if err != nil {
	// 		return err
	// 	}

	// }
	_, err = dm.QueryEntryPoint(`INSERT INTO School (SchoolName, City) VALUES
	('pinole high school', 'san pablo'),
	('richmond high school', 'richmond'),
	('hercules high school', 'hercules');`)
	if err != nil {
		return err
	}

	return nil
}
