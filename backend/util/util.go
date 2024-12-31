package util

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
)

const (
	FRONT_SERVER = ":8080"
)

func SendSql(sql string) (interface{}, error) {
	conn, err := net.Dial("tcp", FRONT_SERVER)
	if err != nil {
		return nil, fmt.Errorf("couldn't stablish connection: %s", err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte(sql))
	if err != nil {
		return nil, fmt.Errorf("couldn't write message: %s", err)
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
			return nil, fmt.Errorf("error reading response: %s", err)
		}

		rawData = append(rawData, buffer[:n]...)
	}

	var jsonPlan interface{}
	err = json.Unmarshal(rawData, &jsonPlan)
	if err != nil {
		return nil, fmt.Errorf("json encoding failted: %s", err)
	}

	return jsonPlan, nil
}
