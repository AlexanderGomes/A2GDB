package engines

import (
	"a2gdb/utils"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Server struct {
	host        string
	port        string
	queryEngine *QueryEngine
}

type Client struct {
	conn        net.Conn
	queryEngine *QueryEngine
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

	fmt.Printf("Server Running On Port: %s\n", server.port)
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Fatal(err)
		}

		client := &Client{
			conn:        conn,
			queryEngine: server.queryEngine,
		}
		go client.handleRequest()
	}
}

func (client *Client) handleRequest() {
	defer client.conn.Close()

	var rawData []byte
	for {
		buffer := make([]byte, 1096)
		n, err := client.conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			}
		}

		rawData = append(rawData, buffer[:n]...)
	}

	err := OperationDecider(rawData, client.queryEngine, client.conn)
	if err != nil {
		fmt.Println("OperationDecider Failed: %w", err)
	}
}

func OperationDecider(req []byte, queryEngine *QueryEngine, conn net.Conn) error {
	operation, data, err := DecodeReq(req)
	if err != nil {
		return fmt.Errorf("DecodeReq failed: %w", err)
	}

	switch operation {
	case REGISTER:
		if err := HandleRegistration(data, queryEngine, conn); err != nil {
			return fmt.Errorf("HandleRegistration Failed: %w", err)
		}
	}

	return nil
}

func HandleRegistration(data []byte, queryEngine *QueryEngine, conn net.Conn) error {
	fmt.Println("called HandleRegistration")
	fields := ParsingRegistration(string(data))
	dbName := fields["dbname"]

	email := fields["email"]
	pass := fields["password"]

	row, err := Bookkeeping(email, pass, dbName, queryEngine)
	if err != nil {
		return fmt.Errorf("HandleRegistration failed: %w", err)
	}

	token, err := Authenticate(row, dbName)
	if err != nil {
		return fmt.Errorf("Authenticate failed: %w", err)
	}

	writeDeadLine := time.Now().Add(5 * time.Second)
	err = conn.SetWriteDeadline(writeDeadLine)
	if err != nil {
		return fmt.Errorf("SetReadDeadline failed: %w", err)
	}

	n, err := conn.Write([]byte(token)) // blocks until somebody reads or a timeout occurs
	if err != nil {
		return fmt.Errorf("conn.Write failed: %w", err)
	}

	if n == 0 {
		return errors.New("network write failed, O bytes written")
	}

	return nil
}

func Authenticate(row *RowV2, dbName string) (string, error) {
	secretKey := []byte(os.Getenv("JWT_SECRET"))
	if len(secretKey) == 0 {
		log.Fatal("JWT_SECRET_KEY environment variable not set or is empty")
	}

	ttl := time.Hour * 1
	expirationTime := time.Now().Add(ttl).Unix()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userId": fmt.Sprintf("%d", row.ID),
		"dbName": dbName,
		"exp":    fmt.Sprintf("%d", expirationTime),
	})

	tokenString, err := token.SignedString(secretKey)
	if err != nil {
		return "", fmt.Errorf("SignedString Failed: %w", err)
	}

	return tokenString, nil
}

func Bookkeeping(email, pass, dbName string, queryEngine *QueryEngine) (*RowV2, error) {
	findSql := fmt.Sprintf("SELECT * FROM `User` WHERE Email = '%s'\n", email)
	encodedPlan, err := utils.SendSql(findSql)
	if err != nil {
		return nil, fmt.Errorf("SendSql failed: %w", err)
	}

	_, _, result := queryEngine.QueryProcessingEntry(encodedPlan, false, false)
	if result.Error != nil {
		return nil, fmt.Errorf("QueryProcessingEntry failed: %w", result.Error)
	}

	if len(result.Rows) > 0 {
		return nil, fmt.Errorf("user already exists")
	}

	sql := fmt.Sprintf("INSERT INTO `User`(Email, Password, DbName) VALUES ('%s', '%s', '%s')\n", email, pass, dbName)
	encodedPlan, err = utils.SendSql(sql)
	if err != nil {
		return nil, fmt.Errorf("SendSql failed: %w", err)
	}

	_, _, result = queryEngine.QueryProcessingEntry(encodedPlan, false, false)
	if result.Error != nil {
		return nil, fmt.Errorf("QueryProcessingEntry failed: %w", result.Error)
	}

	return result.Rows[0], nil
}
