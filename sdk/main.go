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

	cred, err := client.Auth("sander@gmail.com", "81377662", "NEWDB")
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println(cred)
	// schema := map[string]string{"UserId": "PRIMARY KEY", "Name": "VARCHAR", "Email": "VARCHAR", "Password": "VARCHAR"}
	// cred.CreateTable("User", schema)
}
