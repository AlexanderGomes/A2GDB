package main

import (
	"io"
	"log"
	"net"
)

const (
	FRONT_SERVER = ":8080"
)

func main() {
	sql := "SELECT Employees.Name, Departments.DepartmentName FROM Employees JOIN Departments ON Employees.DepartmentID = Departments.DepartmentID AND Departments.DepartmentID = 18281281\n"
	sendSql(sql)
}

func sendSql(sql string) {
	conn, err := net.Dial("tcp", FRONT_SERVER)
	if err != nil {
		log.Println("Couldn't Stablish Connection: ", err)
		return
	}
	defer conn.Close()

	_, err = conn.Write([]byte(sql))
	if err != nil {
		log.Println("Couldn't Write Message: ", err)
		return
	}

	var data []byte

	for {
		buffer := make([]byte, 1024)
		n, err := conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				log.Println("Full Message Read")
				break
			}
			log.Println("Error reading response: ", err)
			return
		}

		data = append(data, buffer[:n]...)
	}

}
