package engines

import (
	"reflect"
	"sync"
)

type MemoryManagerConfig struct {
	TotalMemoryLimit uint64
	DefaultPoolSize  int
}

type MemoryManager struct {
	config           MemoryManagerConfig
	pools            map[reflect.Type]*sync.Pool
	poolsMutex       sync.RWMutex
	totalAllocations uint64
	totalFreed       uint64
	currentMemoryUsage uint64
}

type PoolStats struct {
	TypeName     string
	ObjectSize   uintptr
}

func NewMemoryManager(config MemoryManagerConfig) *MemoryManager {
	if config.DefaultPoolSize == 0 {
		config.DefaultPoolSize = 100 
	}

	if config.TotalMemoryLimit == 0 {
		config.TotalMemoryLimit = 1024 * 1024 * 1024
	}

	return &MemoryManager{
		config: config,
		pools:  make(map[reflect.Type]*sync.Pool),
	}
}


func (mm *MemoryManager) CreatePool(
	objectType reflect.Type,
	allocator func() interface{},
) *sync.Pool {
	mm.poolsMutex.Lock()
	defer mm.poolsMutex.Unlock()

	if existingPool, exists := mm.pools[objectType]; exists {
		return existingPool
	}

	pool := &sync.Pool{
		New: allocator,
	}

	mm.pools[objectType] = pool

	return pool
}

func (mm *MemoryManager) Acquire(objectType reflect.Type) interface{} {
	mm.poolsMutex.RLock()
	defer mm.poolsMutex.RUnlock()

	pool, exists := mm.pools[objectType]
	if !exists {
		return nil
	}

	return pool.Get()
}

func (mm *MemoryManager) Release(objectType reflect.Type, obj interface{}) {
	mm.poolsMutex.RLock()
	defer mm.poolsMutex.RUnlock()

	pool, exists := mm.pools[objectType]
	if !exists {
		return
	}

	pool.Put(obj)
}

func (mm *MemoryManager) GetPoolStats(objectType reflect.Type) *PoolStats {
	mm.poolsMutex.RLock()
	defer mm.poolsMutex.RUnlock()

	objSize := reflect.New(objectType).Elem().Type().Size()

	return &PoolStats{
		TypeName:   objectType.String(),
		ObjectSize: objSize,
	}
}
