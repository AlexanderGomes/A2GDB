package client

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt"
)

const SERVER = ":3404"

func SendBytes(bytes []byte) (net.Conn, error) {
	timeout := 2 * time.Second
	conn, err := net.DialTimeout("tcp", SERVER, timeout)
	if err != nil {
		return nil, fmt.Errorf("couldn't stablish connection: %s", err)
	}

	writeDeadLine := time.Now().Add(4 * time.Second)
	err = conn.SetWriteDeadline(writeDeadLine)
	if err != nil {
		return nil, fmt.Errorf("SetReadDeadline failed: %w", err)
	}

	_, err = conn.Write(bytes)
	if err != nil {
		return nil, fmt.Errorf("couldn't write message: %s", err)
	}

	tcpConn, ok := conn.(*net.TCPConn)
	if ok {
		tcpConn.CloseWrite()
	}

	return conn, nil
}

func (ctcp *CustomTCP) Encode() ([]byte, error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.LittleEndian, ctcp.MessageType); err != nil {
		return nil, err
	}

	if _, err := buf.Write(ctcp.MessageBody); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func ParseToken(tokenBytes []byte) (*UserCred, error) {
	token, err := jwt.Parse(string(tokenBytes), func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(os.Getenv("JWT_SECRET")), nil
	})

	if err != nil {
		errorStr := string(tokenBytes)
		sentence := "incorrect credentials"

		if match := strings.Contains(errorStr, sentence); match {
			return nil, errors.New(sentence)
		}

		return nil, fmt.Errorf("parsing token failed: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.New("JWT token invalid")
	}

	userCred, err := GetClaims(claims)
	if err != nil {
		return nil, fmt.Errorf("GetClaims failed: %w", err)
	}

	return userCred, nil
}

func GetClaims(claims jwt.MapClaims) (*UserCred, error) {
	userIDStr, ok := claims["userId"].(string)
	if !ok {
		return nil, fmt.Errorf("userId is missing or not a string")
	}

	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse userId: %w", err)
	}

	dbName, ok := claims["dbName"].(string)
	if !ok {
		return nil, fmt.Errorf("dbName is missing or not a string")
	}

	return &UserCred{UserId: userID, DbName: dbName}, nil
}
