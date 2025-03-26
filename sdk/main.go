package main

import (
	"fmt"
	"log"
	"sdk/client"
	"sync"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load("../.env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	cred, err := client.Auth("sander@gmail.com", "81377662", "NEWDB")
	if err != nil {
		log.Fatal(err)
	}

	schema := map[string]string{"UserId": "PRIMARY KEY", "Username": "VARCHAR", "Age": "INT", "City": "VARCHAR"}
	cred.CreateTable("User", schema)

	InsertMany(1000, cred)
}

func Concurrent(queries []string, cred *client.UserCred) {
	var wg sync.WaitGroup
	for i := range len(queries) {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			query := queries[i]
			msg, err := cred.ExecuteQuery(query)
			if err != nil {
				panic(err)
			}
			fmt.Printf("Query: %s, Msg: %s\n", query, msg)
		}(i)
	}

	wg.Wait()
}

func InsertMany(x int, cred *client.UserCred) {
	sql := "INSERT INTO `User`(Username, Age, City) VALUES ('JaneSmith', 25, 'Los Angeles'), ('AliceBrown', 28, 'Chicago'), ('BobWhite', 35, 'Houston')"
	for range x {
		msg, err := cred.ExecuteQuery(sql)
		if err != nil {
			panic(err)
		}
		fmt.Println(msg)
	}
}
