package client

import (
	"fmt"
	"io"
	"regexp"
	"time"
)

const (
	AUTH = iota + 1
	CREATE_TABLE
	QUERY
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
	reqBody := "&email=" + email + "&password=" + password + "&dbname=" + dbName + "&"
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

	readDeadLine := time.Now().Add(READ_TIMEOUT * time.Second)
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

func (cred *UserCred) CreateTable(table string, schema map[string]string) error {
	reqBody := fmt.Sprintf("&tableName=%s", table)

	schemaStr := "["
	for key, val := range schema {
		schemaStr += fmt.Sprintf("&%s=%s", key, val)
	}
	schemaStr += "&]"

	reqBody += "&schema=" + schemaStr
	reqBody += "&auth=[" + fmt.Sprintf("&userId=%d&dbName=%s", cred.UserId, cred.DbName)
	reqBody += "&]"

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

	msg, err := ReadResponse(conn)
	if err != nil {
		return fmt.Errorf("ReadResponse Failed: %w", err)
	}

	fmt.Println(msg)

	return nil
}

func (cred *UserCred) GetOfficialTableName(tableName string) string {
	return tableName + "-" + cred.DbName + "-" + fmt.Sprintf("%d", cred.UserId)
}

func (cred *UserCred) ExecuteQuery(sql string) (string, error) {
	var tableName string
	re := regexp.MustCompile("`(.*?)`")
	match := re.FindStringSubmatch(sql)
	if len(match) > 1 {
		tableName = match[1]
	}

	internalTableName := cred.GetOfficialTableName(tableName)

	updatedQuery := re.ReplaceAllString(sql, "`"+internalTableName+"`")
	reqBody := fmt.Sprintf("&sql=%s&", updatedQuery)

	message := CustomTCP{
		MessageType: QUERY,
		MessageBody: []byte(reqBody),
	}

	bytes, err := message.Encode()
	if err != nil {
		return "", fmt.Errorf("encoding tcp failed: %w", err)
	}

	conn, err := SendBytes(bytes)
	if err != nil {
		return "", fmt.Errorf("SendBytes Failed: %w", err)

	}
	defer conn.Close()

	msg, err := ReadResponse(conn)
	if err != nil {
		return "", fmt.Errorf("ReadResponse Failed: %w", err)
	}

	return msg, nil
}
