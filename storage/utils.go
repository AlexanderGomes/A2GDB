package storage

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
)

func GenerateRandomID() uint64 {
	max := new(big.Int).Lsh(big.NewInt(1), 64)
	randomNum, _ := rand.Int(rand.Reader, max)

	return randomNum.Uint64()
}

func Encode(page interface{}) ([]byte, error) {
	encoded, err := json.Marshal(page)
	if err != nil {
		return nil, fmt.Errorf("Encode Error: %w", err)
	}
	return encoded, nil
}

func DecodeV2(page []byte, dst interface{}) error {
	endIndex := bytes.IndexByte(page, 0)
	if endIndex == -1 {
		endIndex = len(page)
	}

	err := json.Unmarshal(page[:endIndex], dst)
	if err != nil {
		return fmt.Errorf("DecodeV2 Error: %w", err)
	}

	return nil
}

func (dm *DiskManagerV2) UpdateCatalog() error {
	bytes, err := Encode(dm.PageCatalog)

	if err != nil {
		return fmt.Errorf("UpdateCatalog: %w", err)
	}

	dm.FileCatalog.WriteAt(bytes, 0)

	return nil
}

type WrapperWaitGroup struct {
	wg    sync.WaitGroup
	count int
	mu    sync.Mutex
}

func (w *WrapperWaitGroup) Add(delta int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.count += delta
	w.wg.Add(delta)
}

func (w *WrapperWaitGroup) Done() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.count--
	w.wg.Done()
}

func (w *WrapperWaitGroup) Wait() {
	w.wg.Wait()
}

func (w *WrapperWaitGroup) Counter() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.count
}
