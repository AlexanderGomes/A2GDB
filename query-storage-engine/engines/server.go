package engines

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
)

type Server struct {
	host string
	port string
}

type Client struct {
	conn net.Conn
}

type Config struct {
	Host string
	Port string
}

func NewServer(config *Config) *Server {
	return &Server{
		host: config.Host,
		port: config.Port,
	}
}

func (server *Server) Run() {
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%s", server.host, server.port))
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Fatal(err)
		}

		client := &Client{
			conn: conn,
		}
		go client.handleRequest()
	}
}

func (client *Client) handleRequest() {
	defer client.conn.Close()

	var rawData []byte
	for {
		buffer := make([]byte, 500)
		n, err := client.conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			}
		}

		rawData = append(rawData, buffer[:n]...)
	}

	if err := Decode(rawData); err != nil {
		fmt.Printf("Deconding failed when handling request: %s", err)
	}
}

func Decode(data []byte) error {
	var operation uint8

	buf := bytes.NewReader(data)

	if err := binary.Read(buf, binary.LittleEndian, &operation); err != nil {
		return err
	}

	remainingData := make([]byte, buf.Len())
	if _, err := buf.Read(remainingData); err != nil {
		return err
	}

	fmt.Printf("Operation: %d, Data: %s", operation, string(remainingData))

	return nil
}
