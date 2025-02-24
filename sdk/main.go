package main

import (
	"fmt"
	"log"
	"sdk/client"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load("../.env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	cred, err := client.Auth("sander@gmail.com", "81377662asaasa", "NEWDB")
	if err != nil {
		fmt.Println(err)
	}

	sql := "INSERT INTO `User` (Name, Email, Password) VALUES ('JaneSmith', 'sander@gmail.com', '199191928182')\n"
	cred.ExecuteQuery(sql)
}
