package client

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	REGISTER = iota + 1
	LOGIN
)

type CustomTCP struct {
	MessageType uint8
	MessageBody []byte
}

type UserCred struct {
	UserId uint64
	DbName string
	TTL    int64
}

func Register(email, password, dbName string) (*UserCred, error) {
	body := "&email=" + email + "&password=" + password + "&dbname=" + dbName + "&"
	message := CustomTCP{
		MessageType: REGISTER,
		MessageBody: []byte(body),
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

	fmt.Println("sent bytes, waiting for response...")

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

func ParseToken(tokenBytes []byte) (*UserCred, error) {
	token, err := jwt.Parse(string(tokenBytes), func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(os.Getenv("JWT_SECRET")), nil
	})

	if err != nil {
		return nil, fmt.Errorf("parsing token failed: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok && !token.Valid {
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

	expStr, ok := claims["exp"].(string)
	if !ok {
		return nil, fmt.Errorf("exp is missing or not a string")
	}

	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse exp: %w", err)
	}

	return &UserCred{UserId: userID, DbName: dbName, TTL: exp}, nil
}
