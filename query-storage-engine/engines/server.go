package engines

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"regexp"
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
		_, err = client.conn.Write([]byte(err.Error()))
		if err != nil {
			fmt.Printf("couldn't network error message: %s\n", err)
		}
	}
}

func OperationDecider(req []byte, queryEngine *QueryEngine, conn net.Conn) error {
	operation, data, err := DecodeReq(req)
	if err != nil {
		return fmt.Errorf("DecodeReq failed: %w", err)
	}

	switch operation {
	case AUTH:
		if err := HandleAuth(data, queryEngine, conn); err != nil {
			return fmt.Errorf("HandleAuth Failed: %w", err)
		}
	case CREATE_TABLE:
		if err := HandleCreateTable(data, queryEngine, conn); err != nil {
			return fmt.Errorf("HandleCreateTable Failed: %w", err)
		}
	case QUERY:
		if err := HandleQueries(data, queryEngine, conn); err != nil {
			return fmt.Errorf("HandleQueries Failed: %w", err)
		}
	}

	return nil
}

func HandleQueries(data []byte, queryEngine *QueryEngine, conn net.Conn) error {
	re := regexp.MustCompile(`=([^&]*)`)
	match := re.FindStringSubmatch(string(data))

	if len(match) > 1 {
		sql := match[1]
		res, err := ExecuteQuery(sql, queryEngine)
		if err != nil {
			return fmt.Errorf("ExecuteQuery Failed: %w", err)
		}

		err = SendResponse(res.Msg, conn)
		if err != nil {
			return fmt.Errorf("SendResponse Failed: %w", err)
		}

	} else {
		return errors.New("request body format incorrect")
	}

	return nil
}

func HandleCreateTable(data []byte, queryEngine *QueryEngine, conn net.Conn) error {
	tableName, fields := ParsingTableMetadata(string(data))
	authMap := fields["auth"]
	schemaMap := fields["schema"]

	dbName := authMap["dbName"]
	userId := authMap["userId"]

	officialTableName := tableName + "-" + dbName + "-" + userId

	schemaStr := CreateSchemaString(schemaMap)
	sql := fmt.Sprintf("CREATE TABLE `%s`(%s)\n", officialTableName, schemaStr)
	res, err := ExecuteQuery(sql, queryEngine)
	if err != nil {
		return fmt.Errorf("ExecuteQuery Failed: %w", err)
	}

	err = SendResponse(res.Msg, conn)
	if err != nil {
		return fmt.Errorf("SendResponse Failed: %w", err)
	}

	return nil
}

func HandleAuth(data []byte, queryEngine *QueryEngine, conn net.Conn) error {
	fields := ParsingRegistration(string(data))

	dbName := fields["dbname"]
	email := fields["email"]
	pass := fields["password"]

	row, err := Bookkeeping(email, pass, dbName, queryEngine)
	if err != nil {
		return fmt.Errorf("Bookkeeping failed: %w", err)
	}

	token, err := Authenticate(row, dbName)
	if err != nil {
		return fmt.Errorf("Authenticate failed: %w", err)
	}

	err = SendResponse(token, conn)
	if err != nil {
		return fmt.Errorf("SendResponse Failed: %w", err)
	}

	return nil
}
