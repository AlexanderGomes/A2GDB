package main

import (
	"disk-db/cmd"
	queryengine "disk-db/query-engine"
	"fmt"
)

const (
	replacerFrequency = 2
	dirName           = "A2G_DB"
)

func main() {
	sql := `SELECT 
    User.UserID,
    User.Username,
    School.SchoolID,
    School.SchoolName
FROM 
    User
JOIN 
    School ON User.City = School.City;
`
	dm, _ := cmd.InitDatabase(replacerFrequency, dirName)

	res, err := dm.QueryEntryPoint(sql)

	if err != nil {
		fmt.Println(err)
	}

	fmt.Println(len(res.Result))
}

func Testing(dm *queryengine.QueryEngine) error {
	var err error

	_, err = dm.QueryEntryPoint(`CREATE TABLE User (
			UserID INT PRIMARY KEY,
			Username VARCHAR,
			Age INT,
			City VARCHAR
);`)

	if err != nil {
		return err
	}

	_, err = dm.QueryEntryPoint(`CREATE TABLE School (
	SchoolID INT PRIMARY KEY,
	SchoolName VARCHAR,
	City VARCHAR
);`)

	if err != nil {
		return err
	}

	for i := 0; i < 1000; i++ {
		_, err = dm.QueryEntryPoint(`INSERT INTO User (Username, Age, City) VALUES
		('sander1', 12, 'san pablo'),
		('sander2', 15, 'san pablo'),
		('sander3', 23, 'richmond'),
		('sander4', 11, 'hercules'),
		('sander5', 7, 'hercules');`)
		if err != nil {
			return err
		}

		_, err = dm.QueryEntryPoint(`INSERT INTO School (SchoolName, City) VALUES
		('pinole high school', 'san pablo'),
		('richmond high school', 'richmond'),
		('hercules high school', 'hercules');`)
		if err != nil {
			return err
		}

	}

	return err
}
