package client

import (
	"fmt"
	"io"
	"time"
)

const (
	AUTH = iota + 1
	CREATE_TABLE
)

type CustomTCP struct {
	MessageType uint8
	MessageBody []byte
}

type UserCred struct {
	UserId uint64
	DbName string
}

func Auth(email, password, dbName string) (*UserCred, error) {
	reqBody := "&email=" + email + "&password=" + password + "&dbname=" + dbName
	message := CustomTCP{
		MessageType: AUTH,
		MessageBody: []byte(reqBody),
	}

	bytes, err := message.Encode()
	if err != nil {
		return nil, fmt.Errorf("encoding tcp failed: %w", err)
	}

	conn, err := SendBytes(bytes)
	if err != nil {
		return nil, fmt.Errorf("SendBytes Failed: %w", err)

	}
	defer conn.Close()

	readDeadLine := time.Now().Add(4 * time.Second)
	err = conn.SetReadDeadline(readDeadLine)
	if err != nil {
		return nil, fmt.Errorf("SetReadDeadline failed: %w", err)
	}

	rawData, err := io.ReadAll(conn)
	if err != nil {
		return nil, fmt.Errorf("io.ReadAll Failed: %w", err)
	}

	credentials, err := ParseToken(rawData)
	if err != nil {
		return nil, fmt.Errorf("ParseToken Failed: %w", err)
	}

	return credentials, nil
}

func (cred *UserCred) CreateTable(tableName string, schema map[string]string) error {
	reqBody := fmt.Sprintf("&name=%s", tableName)

	schemaStr := "["
	for key, val := range schema {
		schemaStr += fmt.Sprintf("&%s=%s", key, val)
	}

	schemaStr += "]"
	reqBody += "&schema=" + schemaStr
	reqBody += "&auth=[" + fmt.Sprintf("&userId=%d&dbName=%s", cred.UserId, cred.DbName)
	reqBody += "]"

	message := CustomTCP{
		MessageType: CREATE_TABLE,
		MessageBody: []byte(reqBody),
	}

	bytes, err := message.Encode()
	if err != nil {
		return fmt.Errorf("encoding tcp failed: %w", err)
	}

	conn, err := SendBytes(bytes)
	if err != nil {
		return fmt.Errorf("SendBytes Failed: %w", err)

	}
	defer conn.Close()

	return nil
}
