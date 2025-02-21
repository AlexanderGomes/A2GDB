package client

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"time"
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
