package storage

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

type LogType uint8

const (
	LogTypeInsert LogType = iota + 1
	LogTypeUpdate
	LogTypeDelete
	LogTypeCommit
	LogTypeAbort
	LogTypeCheckpoint
)

type LogRecord struct {
	LSN         uint64
	Type        LogType
	TxID        string
	TableID     string
	RowID       uint64
	BeforeImage []byte
	AfterImage  []byte
	Timestamp   time.Time
}

type WalManager struct {
	logBuffer   chan *LogRecord
	CurrentLSN  uint64
	writer      *bufio.Writer
	file        *os.File
	flushTicker *time.Ticker
	mu          sync.Mutex
}

func NewWalManager(logFile string) (*WalManager, error) {
	file, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	writer := bufio.NewWriter(file)

	wm := &WalManager{
		logBuffer: make(chan *LogRecord, 1000),
		file:      file,
		writer:    writer,
	}

	return wm, nil
}

func (wl *WalManager) Log(query LogType, tableName string, rowId uint64, beforeImg, afterImg []byte) error {
	wl.mu.Lock()
	defer wl.mu.Unlock()

	wl.CurrentLSN++
	record := LogRecord{
		LSN:         wl.CurrentLSN,
		TxID:        strconv.FormatUint(GenerateRandomID(), 10),
		Type:        query,
		TableID:     tableName,
		RowID:       rowId,
		BeforeImage: beforeImg,
		AfterImage:  afterImg,
		Timestamp:   time.Now(),
	}

	bytes, err := encodeLog(&record)
	if err != nil {
		return fmt.Errorf("encodeLog failed: %w", err)
	}

	_, err = wl.writer.Write(bytes)
	if err != nil {
		return fmt.Errorf("log - writer failed: %w", err)
	}

	// testing purposes // shouldn't flush after every write
	if err := wl.writer.Flush(); err != nil {
		return fmt.Errorf("log - flush failed: %w", err)
	}

	return nil
}
