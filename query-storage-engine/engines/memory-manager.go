package engines

import (
	"reflect"
	"sync"
	"sync/atomic"
	"unsafe"
)

type MemoryContextType int

const (
	Insert MemoryContextType = iota + 1
	UpdateMultiple
	UpdateSingle
	DeleteSingle
	DeleteMultiple
	SelectStarAndColumn
	SelectWhereClause
	SelectOrderByClause
	SelectGroupByClauseCount
)

type AllocationStrategy int

const (
	DefaultAllocation AllocationStrategy = iota + 1
	SmallObjectAllocation
	MediumObjectAllocation
	LargeObjectAllocation
)

type ContextManager struct {
	mu       sync.RWMutex
	ctxCache map[MemoryContextType][]*MemoryContext
}

type MemoryContext struct {
	active bool
	mu     sync.RWMutex

	name     string
	parent   *MemoryContext
	children []*MemoryContext
	pools    map[reflect.Type]Pool

	stats *MemContextStats

	contextType   MemoryContextType
	allocStrategy AllocationStrategy
}

type MemContextStats struct {
	totalAllocated uint64
	totalFreed     uint64
	peakUsage      uint64
	currentUsage   uint64
}

type MemoryContextConfig struct {
	Name            string
	Parent          *MemoryContext
	ContextType     MemoryContextType
	AllocationStrat AllocationStrategy
}

type Pool struct {
	allocator func() any
	mu        *sync.Mutex
	pool      []any
	capacity  int
}

func NewMemoryContext(config MemoryContextConfig) *MemoryContext {
	mc := &MemoryContext{
		name:          config.Name,
		parent:        config.Parent,
		contextType:   config.ContextType,
		allocStrategy: config.AllocationStrat,
		pools:         make(map[reflect.Type]Pool),
		stats:         &MemContextStats{},
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

func (mc *MemoryContext) CreatePool(objectType reflect.Type, allocator func() any, capacity int) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	poolObj := Pool{
		pool:      make([]any, 0, capacity),
		capacity:  capacity,
		mu:        &sync.Mutex{},
		allocator: allocator,
	}

	for range capacity {
		obj := allocator()
		size := unsafe.Sizeof(obj)

		atomic.AddUint64(&mc.stats.totalAllocated, uint64(size))
		atomic.AddUint64(&mc.stats.currentUsage, uint64(size))

		for {
			current := atomic.LoadUint64(&mc.stats.currentUsage)
			peak := atomic.LoadUint64(&mc.stats.peakUsage)
			if current <= peak {
				break
			}
			if atomic.CompareAndSwapUint64(&mc.stats.peakUsage, peak, current) {
				break
			}
		}

		poolObj.pool = append(poolObj.pool, obj)
	}

	mc.pools[objectType] = poolObj
}

func (mm *MemoryContext) Acquire(objectType reflect.Type) any {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	poolObj, exists := mm.pools[objectType]
	if !exists || len(poolObj.pool) == 0 {
		return nil
	}

	poolObj.mu.Lock()
	defer poolObj.mu.Unlock()

	if len(poolObj.pool) == 0 {
		Double(&poolObj)
	}

	obj := poolObj.pool[len(poolObj.pool)-1]
	poolObj.pool = poolObj.pool[:len(poolObj.pool)-1]

	mm.pools[objectType] = poolObj

	size := unsafe.Sizeof(obj)

	atomic.AddUint64(&mm.stats.totalAllocated, uint64(size))
	current := atomic.AddUint64(&mm.stats.currentUsage, uint64(size))

	for {
		peak := atomic.LoadUint64(&mm.stats.peakUsage)
		if current <= peak {
			break
		}
		if atomic.CompareAndSwapUint64(&mm.stats.peakUsage, peak, current) {
			break
		}
	}

	return obj
}

func (mm *MemoryContext) Release(objectType reflect.Type, obj any) bool {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	poolObj, exists := mm.pools[objectType]
	if !exists {
		return false
	}

	poolObj.mu.Lock()
	defer poolObj.mu.Unlock()

	clearObjectFields(obj)
	poolObj.pool = append(poolObj.pool, obj)
	mm.pools[objectType] = poolObj

	// x == capacity means everybody returned their objects
	// shrink
	if len(poolObj.pool) == poolObj.capacity && poolObj.capacity > 2 {
		Shrink(&poolObj)
	}

	size := unsafe.Sizeof(obj)
	atomic.AddUint64(&mm.stats.totalFreed, uint64(size))
	atomic.AddUint64(&mm.stats.currentUsage, -uint64(size))

	return true
}

func clearObjectFields(obj any) {
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

func MemorySnap(rootCtx *MemoryContext) *MemContextStats {
	var s MemContextStats

	rootCtx.mu.RLock()
	defer rootCtx.mu.RUnlock()

	CollectStats(rootCtx.stats, &s)

	for _, child := range rootCtx.children {
		childStats(child, &s)
	}

	return &s
}

func childStats(ctx *MemoryContext, destination *MemContextStats) {
	if ctx == nil {
		return
	}

	ctx.mu.RLock()

	CollectStats(ctx.stats, destination)

	children := make([]*MemoryContext, len(ctx.children))
	copy(children, ctx.children)

	ctx.mu.RUnlock()

	for _, child := range children {
		childStats(child, destination)
	}
}

func CollectStats(source, destination *MemContextStats) {
	destination.currentUsage += source.currentUsage
	destination.totalAllocated += source.totalAllocated
	destination.totalFreed += source.totalFreed
	destination.peakUsage += source.peakUsage
}

func Double(poolObj *Pool) {
	newCapacity := poolObj.capacity * 2
	poolObj.capacity = newCapacity

	newPool := make([]any, len(poolObj.pool), newCapacity)
	copy(newPool, poolObj.pool)

	for range newCapacity {
		newObj := poolObj.allocator()
		newPool = append(newPool, newObj)
	}

	poolObj.pool = newPool
}

func Shrink(poolObj *Pool) {
	if poolObj.capacity <= 1 {
		return
	}

	newCapacity := poolObj.capacity / 2

	newPool := make([]any, newCapacity)
	newPool = append(newPool, poolObj.pool[:newCapacity]...)

	poolObj.capacity = newCapacity
	poolObj.pool = newPool
}

// if cache type isn't cached, the method will return create a new context.
func (cm *ContextManager) GetOrCreateContext(ctxType MemoryContextType, config MemoryContextConfig) *MemoryContext {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	var ctx *MemoryContext

	ctxs, ok := cm.ctxCache[ctxType]
	if !ok {
		return nil
	}

	if len(ctxs) == 0 {
		ctx = NewMemoryContext(config)
		ctx.active = true
		return ctx
	}

	ctx = ctxs[0]
	ctx.active = true

	cm.ctxCache[ctxType] = ctxs[1:]

	return ctx
}

// caches the memory context structure for similar queries
func (cm *ContextManager) ReturnContext(rootCtx *MemoryContext) {
	CleanInfoAndPool(rootCtx)

	for _, child := range rootCtx.children {
		CleanAllChildren(child)
	}

	rootCtx.mu.Lock()
	rootCtx.active = false
	rootCtx.mu.Unlock()

	cm.mu.Lock()
	cm.ctxCache[rootCtx.contextType] = append(cm.ctxCache[rootCtx.contextType], rootCtx)
	cm.mu.Unlock()
}

func CleanAllChildren(childCtx *MemoryContext) {
	CleanInfoAndPool(childCtx)

	for _, child := range childCtx.children {
		CleanAllChildren(child)
	}

}

func CleanInfoAndPool(ctx *MemoryContext) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	ctx.stats.currentUsage = 0
	ctx.stats.peakUsage = 0
	ctx.stats.totalAllocated = 0
	ctx.stats.totalFreed = 0

	ctx.name = ""
	ctx.contextType = 0
	ctx.allocStrategy = 0

	for _, poolObj := range ctx.pools {
		poolObj.mu.Lock()
		poolObj.allocator = nil
		Shrink(&poolObj)
		poolObj.mu.Unlock()
	}

}
