package storage

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
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
