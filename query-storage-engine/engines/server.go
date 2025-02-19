package engines

import (
	"fmt"
	"io"
	"log"
	"net"
)

type Server struct {
	host        string
	port        string
	queryEngine *QueryEngine
}

type Client struct {
	conn net.Conn
}

type Config struct {
	Host        string
	Port        string
	QueryEngine *QueryEngine
}

func NewServer(config *Config) *Server {
	return &Server{
		host:        config.Host,
		port:        config.Port,
		queryEngine: config.QueryEngine,
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
		go client.handleRequest(server)
	}
}

func (client *Client) handleRequest(server *Server) {
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

	err := OperationDecider(rawData, server)
	if err != nil {
		fmt.Println("OperationDecider Failed: %w", err)
	}
}

func (server *Server) HandleRegistration(data []byte) error {
	fields := ParsingRegistration(string(data))
	dbName := fields["dbname"]
	futureFilePath := fmt.Sprintf("A2G_DB_OS/Dbs/%s", dbName)

	if _, err := CreatDefaultManager(futureFilePath); err != nil {
		return fmt.Errorf("CreatDefaultManager failed: %w", err)
	}

	email := fields["email"]
	pass := fields["password"]

	sql := fmt.Sprintf("INSERT INTO `User`(Email, Password, DbPath) VALUES ('%s', '%s', '%s')\n", email, pass, futureFilePath)
	fmt.Println("sql: ", sql)
	encodedPlan, err := SendSql(sql)
	if err != nil {
		return fmt.Errorf("SendSql failed: %w", err)
	}

	fmt.Println("encodedPlan: ", encodedPlan)

	_, _, result := server.queryEngine.QueryProcessingEntry(encodedPlan, false, false)

	fmt.Println("Result: ", result)
	if result.Error != nil {
		return fmt.Errorf("QueryProcessingEntry failed: %w", err)
	}

	return nil
}
