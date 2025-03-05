package engines

import (
	"sync"
)

const (
	W = "WRITE"
	R = "READ"
)

type LockManager struct {
	Mu   sync.RWMutex
	Rows map[uint64]*RowInfo
}

type RowInfo struct {
	Mu     sync.RWMutex
	RowPtr *RowV2
}

func (lm *LockManager) Lock(rowId uint64, row *RowV2, lockType string) {
	mu := lm.GetOrSetLocker(rowId, row)

	if lockType == R {
		mu.RLock()
		return
	}

	mu.Lock()
}

func (lm *LockManager) Unlock(rowId uint64, row *RowV2, lockType string) error {
	mu := lm.GetOrSetLocker(rowId, row)

	if lockType == R {
		mu.RUnlock()
		return nil
	}

	mu.Unlock()
	return nil
}

func (lm *LockManager) GetOrSetLocker(rowId uint64, row *RowV2) *sync.RWMutex {
	lm.Mu.Lock()
	defer lm.Mu.Unlock()

	rowInfo := lm.GetRowInfo(rowId)
	if rowInfo == nil {
		rowInfo = &RowInfo{Mu: sync.RWMutex{}, RowPtr: row}
		lm.SetRow(rowId, rowInfo)
	}

	return &rowInfo.Mu
}

func (lm *LockManager) SetRow(rowId uint64, row *RowInfo) {
	lm.Rows[rowId] = row
}

func (lm *LockManager) GetRowInfo(rowId uint64) *RowInfo {
	if row, ok := lm.Rows[rowId]; ok {
		return row
	}
	return nil
}
