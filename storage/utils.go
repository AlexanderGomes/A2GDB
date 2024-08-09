package storage

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"unsafe"
)

func generateRandomID() uint64 {
	max := new(big.Int).Lsh(big.NewInt(1), 64)
	randomNum, _ := rand.Int(rand.Reader, max)

	return randomNum.Uint64()
}

func getSizeOfIDAndRows(page *Page) uintptr {
	size := unsafe.Sizeof(page.ID)

	size += unsafe.Sizeof(page.Rows)

	for k, v := range page.Rows {
		size += unsafe.Sizeof(k)
		size += unsafe.Sizeof(v)

		for key, value := range v.Values {
			size += unsafe.Sizeof(key)
			size += uintptr(len(key))
			size += unsafe.Sizeof(value)
			size += uintptr(len(value))
		}
	}

	return size
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

func SerializeTuple(t Tuple) []byte {
	var buf bytes.Buffer

	err := binary.Write(&buf, binary.LittleEndian, t.Header.Length)
	if err != nil {
		fmt.Println("Failed to write Length:", err)
		return nil
	}

	err = binary.Write(&buf, binary.LittleEndian, t.Header.Flags)
	if err != nil {
		fmt.Println("Failed to write Flags:", err)
		return nil
	}

	// Serialize the Data
	_, err = buf.Write(t.Data)
	if err != nil {
		fmt.Println("Failed to write Data:", err)
		return nil
	}

	return buf.Bytes()
}

func SerializePage(p *PageV2) ([]byte, error) {
	var buf bytes.Buffer

	// Write Header
	if err := binary.Write(&buf, binary.LittleEndian, p.Header); err != nil {
		return nil, err
	}

	// Write PointerArray
	for _, loc := range p.PointerArray {
		if err := binary.Write(&buf, binary.LittleEndian, loc); err != nil {
			return nil, err
		}
	}

	// Write Data
	if _, err := buf.Write(p.Data); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
