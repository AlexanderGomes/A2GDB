package client

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
)

const SERVER = ":8080"

func SendBytes(bytes []byte) error {
	conn, err := net.Dial("tcp", SERVER)
	if err != nil {
		return fmt.Errorf("couldn't stablish connection: %s", err)
	}
	defer conn.Close()

	_, err = conn.Write(bytes)
	if err != nil {
		return fmt.Errorf("couldn't write message: %s", err)
	}

	return nil
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
