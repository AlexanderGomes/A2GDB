package main

import (
	"a2gdb/cmd"
	"a2gdb/engines"
	"log"
)

func main() {
	engine, err := cmd.InitDatabase(2, "A2G_DB_OS")
	if err != nil {
		log.Fatal("DB init failed: ", err)
	}

	config := engines.Config{Host: "localhost", Port: "8080", QueryEngine: engine}
	server := engines.NewServer(&config)
	server.Run()
}
