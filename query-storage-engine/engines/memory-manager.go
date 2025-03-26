package engines

import (
	"reflect"
	"sync"
	"sync/atomic"
	"unsafe"
)

type MemoryContextType int

const (
	TopMemoryContext MemoryContextType = iota
	TransientMemoryContext
	QueryMemoryContext
	PerTupleMemoryContext
)

type AllocationStrategy int

const (
	DefaultAllocation AllocationStrategy = iota
	SmallObjectAllocation
	LargeObjectAllocation
)

type MemoryContext struct {
	mu   sync.RWMutex
	name string

	parent   *MemoryContext
	children []*MemoryContext

	contextType   MemoryContextType
	allocStrategy AllocationStrategy

	totalAllocated uint64
	totalFreed     uint64
	peakUsage      uint64
	currentUsage   uint64

	pools map[reflect.Type]*sync.Pool
}

type MemoryContextConfig struct {
	Name            string
	Parent          *MemoryContext
	ContextType     MemoryContextType
	AllocationStrat AllocationStrategy
}

func NewMemoryContext(config MemoryContextConfig) *MemoryContext {
	mc := &MemoryContext{
		name:          config.Name,
		parent:        config.Parent,
		contextType:   config.ContextType,
		allocStrategy: config.AllocationStrat,
		pools:         make(map[reflect.Type]*sync.Pool),
	}

	if mc.parent != nil {
		mc.parent.registerChild(mc)
	}

	return mc
}

// Creates and registers a child context with the same
// context type as the parent.
func (mc *MemoryContext) CreateChild(name string) {
	memCtx := mc.Allocate(name)
	mc.registerChild(memCtx)
}

func (mc *MemoryContext) Allocate(name string) *MemoryContext {
	return NewMemoryContext(MemoryContextConfig{
		Name:        name,
		Parent:      mc,
		ContextType: mc.contextType,
	})
}

// Register a child with custom contex type, not the same
// as its parent
func (mc *MemoryContext) registerChild(child *MemoryContext) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.children = append(mc.children, child)
}

func (mc *MemoryContext) CreatePool(
	objectType reflect.Type,
	allocator func() interface{},
) *sync.Pool {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	pool := &sync.Pool{
		New: func() interface{} {
			obj := allocator()
			size := unsafe.Sizeof(obj)
			atomic.AddUint64(&mc.totalAllocated, uint64(size))
			atomic.AddUint64(&mc.currentUsage, uint64(size))

			for {
				current := atomic.LoadUint64(&mc.currentUsage)
				peak := atomic.LoadUint64(&mc.peakUsage)
				if current <= peak {
					break
				}
				if atomic.CompareAndSwapUint64(&mc.peakUsage, peak, current) {
					break
				}
			}

			return obj
		},
	}

	mc.pools[objectType] = pool
	return pool
}

func (mm *MemoryContext) Acquire(objectType reflect.Type) interface{} {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	pool, exists := mm.pools[objectType]
	if !exists {
		return nil
	}

	obj := pool.Get()
	size := unsafe.Sizeof(obj)

	atomic.AddUint64(&mm.totalAllocated, uint64(size))
	current := atomic.AddUint64(&mm.currentUsage, uint64(size))

	for {
		peak := atomic.LoadUint64(&mm.peakUsage)
		if current <= peak {
			break
		}
		if atomic.CompareAndSwapUint64(&mm.peakUsage, peak, current) {
			break
		}
	}

	return obj
}

func (mc *MemoryContext) Release(objectType reflect.Type, obj interface{}) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	pool, exists := mc.pools[objectType]
	if !exists {
		return
	}

	clearObjectFields(obj)

	size := unsafe.Sizeof(obj)
	atomic.AddUint64(&mc.totalFreed, uint64(size))
	atomic.AddUint64(&mc.currentUsage, ^uint64(size-1))

	pool.Put(obj)
}

func clearObjectFields(obj interface{}) {
	v := reflect.ValueOf(obj)

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return
	}

	for i := range v.NumField() {
		field := v.Field(i)

		if !field.CanSet() {
			panic("unaddressable field")
		}

		switch field.Kind() {
		case reflect.Slice:
			field.Set(reflect.Zero(field.Type()))
		case reflect.Map:
			field.Set(reflect.Zero(field.Type()))
		case reflect.Ptr:
			field.Set(reflect.Zero(field.Type()))
		case reflect.String:
			field.SetString("")
		case reflect.Bool:
			field.SetBool(false)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			field.SetInt(0)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			field.SetUint(0)
		case reflect.Float32, reflect.Float64:
			field.SetFloat(0)
		case reflect.Struct:
			clearObjectFields(field.Addr().Interface())
		}
	}
}
