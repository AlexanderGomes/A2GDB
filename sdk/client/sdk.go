package client

import (
	"fmt"
)

const (
	REGISTER = iota + 1
	LOGIN
)

type CustomTCP struct {
	MessageType uint8
	MessageBody []byte
}

func Register(email, password, dbName string) error {
	body := "&email=" + email + "&password=" + password + "&dbname=" + dbName + "&"
	message := CustomTCP{
		MessageType: REGISTER,
		MessageBody: []byte(body),
	}

	bytes, err := message.Encode()
	if err != nil {
		return fmt.Errorf("encoding tcp failed: %w", err)
	}

	if err := SendBytes(bytes); err != nil {
		return fmt.Errorf("SendBytes Failed: %w", err)
	}

	return nil
}
