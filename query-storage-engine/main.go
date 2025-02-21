package main

import (
	"a2gdb/cmd"
	"a2gdb/engines"
	"github.com/joho/godotenv"
	"log"
)

func main() {
	err := godotenv.Load("../.env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	engine, err := cmd.InitDatabase(2, "A2G_DB_OS")
	if err != nil {
		log.Fatal("DB init failed: ", err)
	}

	config := engines.Config{Host: "localhost", Port: "3404", QueryEngine: engine}
	server := engines.NewServer(&config)
	server.Run()
}
