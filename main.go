package main

import (
	"disk-db/cmd"
	queryengine "disk-db/query-engine"
)

const (
	replacerFrequency = 2
	dirName           = "A2G_DB"
)

func main() {
	dm, _ := cmd.InitDatabase(replacerFrequency, dirName)

	Testing(dm)
}

func Testing(dm *queryengine.QueryEngine) error {
	var err error

	_, err = dm.QueryEntryPoint(`CREATE TABLE User (
			UserID INT PRIMARY KEY,
			Username VARCHAR,
			Age INT,
			City VARCHAR
);`)

	for i := 0; i < 1000; i++ {
		_, err = dm.QueryEntryPoint(`INSERT INTO User (Username, Age, City) VALUES
		('sander1', 12, 'richmond'),
		('sander2', 15, 'richmond'),
		('sander3', 23, 'richmond'),
		('sander4', 11, 'richmond'),
		('sander5', 7, 'richmond');`)
		if err != nil {
			return err
		}

		_, err = dm.QueryEntryPoint(`INSERT INTO User (Username, Age, City) VALUES
		('sander6', 58, 'san pablo'),
		('sander7', 77, 'san pablo'),
		('sander8', 31, 'san pablo'),
		('sander9', 21, 'san pablo'),
		('sander10', 93, 'san pablo');`)
		if err != nil {
			return err
		}

		_, err = dm.QueryEntryPoint(`INSERT INTO User (Username, Age, City) VALUES
		('sander11', 16, 'pinole'),
		('sander12', 25, 'pinole'),
		('sander13', 11, 'pinole'),
		('sander14', 12, 'pinole'),
		('sander15', 10, 'pinole');`)
		if err != nil {
			return err
		}

		_, err = dm.QueryEntryPoint(`INSERT INTO User (Username, Age, City) VALUES
		('sander16', 90, 'san francisco'),
		('sander17', 97, 'san francisco'),
		('sander18', 93, 'san francisco'),
		('sander19', 95, 'san francisco'),
		('sander20', 91, 'san francisco');`)
		if err != nil {
			return err
		}
	}

	return err
}
