package engines

import (
	"bufio"
	"fmt"
	"io"
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
	currentLSN    uint64
	writer        *bufio.Writer
	file          *os.File
	activeTx      map[string][]*LogRecord
	activeTxTable map[string]*Table
	mu            sync.Mutex
}

type Table struct {
	notification chan bool
	activeTx     bool
}

func NewWalManager(logFile string) (*WalManager, error) {
	file, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	writer := bufio.NewWriter(file)

	wm := &WalManager{
		file:          file,
		writer:        writer,
		activeTx:      make(map[string][]*LogRecord),
		activeTxTable: make(map[string]*Table),
	}

	err = wm.getLSN()
	if err != nil {
		return nil, fmt.Errorf("getLSN Failed: %w", err)
	}

	return wm, nil
}

func (wl *WalManager) Log(txID string, query LogType, tableName string, rowId uint64, beforeImg, afterImg []byte) error {
	wl.mu.Lock()
	defer wl.mu.Unlock()

	if _, exists := wl.activeTx[txID]; !exists {
		return fmt.Errorf("transaction %s not found", txID)
	}

	wl.currentLSN++
	record := LogRecord{
		LSN:         wl.currentLSN,
		TxID:        txID,
		Type:        query,
		TableID:     tableName,
		RowID:       rowId,
		BeforeImage: beforeImg,
		AfterImage:  afterImg,
		Timestamp:   time.Now(),
	}

	wl.activeTx[txID] = append(wl.activeTx[txID], &record)

	bytes, err := encodeLog(&record)
	if err != nil {
		return fmt.Errorf("encodeLog failed: %w", err)
	}

	_, err = wl.writer.Write(bytes)
	if err != nil {
		return fmt.Errorf("log - writer failed: %w", err)
	}

	return nil
}

func (wl *WalManager) CommitTransaction(txID string, tableName string) error {
	wl.mu.Lock()
	defer wl.mu.Unlock()

	if _, exists := wl.activeTx[txID]; !exists {
		return fmt.Errorf("transaction %s not found", txID)
	}

	wl.currentLSN++
	commitRecord := LogRecord{
		LSN:       wl.currentLSN,
		TxID:      txID,
		Type:      LogTypeCommit,
		Timestamp: time.Now(),
	}

	bytes, err := encodeLog(&commitRecord)
	if err != nil {
		return fmt.Errorf("encodeLog failed: %w", err)
	}

	_, err = wl.writer.Write(bytes)
	if err != nil {
		return fmt.Errorf("commit - writer failed: %w", err)
	}

	err = wl.writer.Flush()
	if err != nil {
		return fmt.Errorf("commit - flush failed: %w", err)
	}

	delete(wl.activeTx, txID)
	tableInfo := wl.activeTxTable[tableName]
	tableInfo.activeTx = false

	select {
	case tableInfo.notification <- true:
	default:
		fmt.Println("No crud waiting")
	}

	return nil
}

func (wl *WalManager) AbortTransaction(txID, primary, modifiedColumn, tableName string, engine *QueryEngine, catalog *Catalog) error {
	wl.mu.Lock()
	defer wl.mu.Unlock()

	logs, exists := wl.activeTx[txID]
	if !exists {
		return fmt.Errorf("transaction %s not found", txID)
	}

	if err := Undo(logs, engine, catalog, primary, modifiedColumn); err != nil {
		return fmt.Errorf("undo failed: %w", err)
	}

	wl.currentLSN++
	abortRecord := LogRecord{
		LSN:       wl.currentLSN,
		TxID:      txID,
		Type:      LogTypeAbort,
		Timestamp: time.Now(),
	}

	bytes, err := encodeLog(&abortRecord)
	if err != nil {
		return fmt.Errorf("encodeLog failed: %w", err)
	}

	_, err = wl.writer.Write(bytes)
	if err != nil {
		return fmt.Errorf("abort - writer failed: %w", err)
	}

	err = wl.writer.Flush()
	if err != nil {
		return fmt.Errorf("abort - flush failed: %w", err)
	}

	delete(wl.activeTx, txID)
	tableInfo := wl.activeTxTable[tableName]
	tableInfo.activeTx = false

	select {
	case tableInfo.notification <- true:
	default:
		fmt.Println("No crud waiting")
	}

	return nil
}

func Undo(logs []*LogRecord, engine *QueryEngine, catalog *Catalog, primary, modifiedColumn string) error {
	for i := len(logs) - 1; i >= 0; i-- {
		log := logs[i]
		switch log.Type {
		case LogTypeInsert:
			err := undoInsert(log, engine, primary)
			if err != nil {
				return fmt.Errorf("undoInsert failed: %w", err)
			}
		case LogTypeUpdate:
			err := undoUpdate(log, engine, primary, modifiedColumn)
			if err != nil {
				return fmt.Errorf("undoUpdate failed: %w", err)
			}
		case LogTypeDelete:
			err := undoDelete(log, engine, catalog)
			if err != nil {
				return fmt.Errorf("undoDelete failed: %w", err)
			}
		}
	}

	return nil
}

func Redo(logs []*LogRecord, engine *QueryEngine, catalog *Catalog, primary, modifiedColumn string) error {
	for _, log := range logs {
		switch log.Type {
		case LogTypeInsert:
			err := redoInsert(log, engine)
			if err != nil {
				return fmt.Errorf("redoInsert failed: %w", err)
			}
		case LogTypeUpdate:
			err := redoUpdate(log, engine)
			if err != nil {
				return fmt.Errorf("redoUpdate failed: %w", err)
			}
		case LogTypeDelete:
			err := redoDelete(log, engine, catalog)
			if err != nil {
				return fmt.Errorf("redoDelete failed: %w", err)
			}
		}
	}

	return nil
}

func (wl *WalManager) getLSN() error {
	wl.mu.Lock()
	defer wl.mu.Unlock()

	fileInfo, err := wl.file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	fileSize := fileInfo.Size()

	if fileSize == 0 {
		return nil
	}

	var offset int64
	prefix := 4
	for {
		tempBuffer := make([]byte, prefix)
		_, err := wl.file.ReadAt(tempBuffer, offset)
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("reading from file (failed): %w", err)
		}

		length, err := decodeLength(tempBuffer)
		if err != nil {
			return fmt.Errorf("decodeLength failed: %w", err)
		}

		logLocation := int64(length) + int64(prefix)

		if (offset + logLocation) == fileSize {
			tempBuffer := make([]byte, length)
			_, err := wl.file.ReadAt(tempBuffer, offset+int64(prefix))
			if err != nil {
				return fmt.Errorf("reading from file (failed): %w", err)
			}

			log, err := deserializeLogRecord(tempBuffer)
			if err != nil {
				return fmt.Errorf("deserializeLogRecord failed: %w", err)
			}

			wl.currentLSN = log.LSN
			break
		}

		offset += logLocation

	}

	return nil
}

func (wl *WalManager) BeginTransaction() string {
	wl.mu.Lock()
	defer wl.mu.Unlock()

	txID := strconv.FormatUint(GenerateRandomID(), 10)
	wl.activeTx[txID] = []*LogRecord{}
	return txID
}
