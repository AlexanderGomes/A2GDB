package util

import (
	"encoding/json"
	"io"
	"log"
	"net"
)

const (
	FRONT_SERVER = ":8080"
)

func SendSql(sql string) interface{} {
	conn, err := net.Dial("tcp", FRONT_SERVER)
	if err != nil {
		log.Println("Couldn't Stablish Connection: ", err)
		return ""
	}
	defer conn.Close()

	_, err = conn.Write([]byte(sql))
	if err != nil {
		log.Println("Couldn't Write Message: ", err)
		return ""
	}

	var rawData []byte
	for {
		buffer := make([]byte, 1024)
		n, err := conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				log.Println("Full Message Read")
				break
			}
			log.Println("Error reading response: ", err)
			return ""
		}

		rawData = append(rawData, buffer[:n]...)
	}

	var jsonPlan interface{}
	err = json.Unmarshal(rawData, &jsonPlan)
	if err != nil {
		log.Println("Json encoding failted: ", err)
		return ""
	}

	return jsonPlan
}
