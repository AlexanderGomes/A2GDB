package engines

import "sync"

const (
	W = "WRITE"
	R = "READ"
)

type RowId uint64
type LockManager struct {
	Mu   sync.RWMutex
	Rows map[RowId]*RowInfo
}

type RowInfo struct {
	Mu     *sync.RWMutex
	RowPtr *RowV2
	Active bool
}

func (lm *LockManager) Lock(id RowId, row *RowV2, lockType string) {
	mu, rowInfo := lm.GetOrSetLocker(id, row)
	rowInfo.Active = true

	if lockType == R {
		mu.RLock()
		return
	}
	mu.Lock()
}

func (lm *LockManager) Unlock(id RowId, row *RowV2, lockType string) {
	mu, rowInfo := lm.GetOrSetLocker(id, row)
	rowInfo.Active = false

	if lockType == R {
		mu.RUnlock()
		return
	}
	mu.Unlock()
}

func (lm *LockManager) GetOrSetLocker(id RowId, row *RowV2) (*sync.RWMutex, *RowInfo) {
	rowInfo := lm.GetRowInfo(id)
	if rowInfo == nil {
		rowInfo = &RowInfo{Mu: &sync.RWMutex{}, RowPtr: row}
		lm.SetRow(id, rowInfo)
	}

	return rowInfo.Mu, rowInfo
}

func (lm *LockManager) SetRow(id RowId, row *RowInfo) {
	lm.Mu.Lock()
	defer lm.Mu.Unlock()
	lm.Rows[id] = row
}

func (lm *LockManager) GetRowInfo(id RowId) *RowInfo {
	lm.Mu.RLock()
	defer lm.Mu.RUnlock()
	if row, ok := lm.Rows[id]; ok {
		return row
	}
	return nil
}
