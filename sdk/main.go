package main

import (
	"log"
	"sdk/client"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load("../.env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	credentials, err := client.Register("sander@gmail.com", "81377662", "akaksk")
	if err != nil {
		panic(err)
	}

}
