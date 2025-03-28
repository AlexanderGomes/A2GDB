package engines

import (
	"reflect"
	"sync"
	"sync/atomic"
	"unsafe"
)

type MemoryContextType int

const (
	TupleLevel MemoryContextType = iota + 1
	AccountingLevel
)

type AllocationStrategy int

const (
	DefaultAllocation AllocationStrategy = iota + 1
	SmallObjectAllocation
	MediumObjectAllocation
	LargeObjectAllocation
)

type ContextManager struct {
	mu       *sync.RWMutex
	ctxCache map[MemoryContextType][]*MemoryContext
}

func NewContextManager() *ContextManager {
	return &ContextManager{
		mu:       &sync.RWMutex{},
		ctxCache: map[MemoryContextType][]*MemoryContext{},
	}
}

// if cache type isn't cached, the method will return create a new context.
func (cm *ContextManager) GetOrCreateContext(ctxType MemoryContextType, config MemoryContextConfig) (*MemoryContext, bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	var ctx *MemoryContext
	var wasCached bool

	ctxs, ok := cm.ctxCache[ctxType]
	if len(ctxs) == 0 || !ok {
		ctx = NewMemoryContext(config)
		ctx.active = true

		return ctx, wasCached
	}

	ctx = ctxs[0]
	ctx.active = true

	cm.ctxCache[ctxType] = ctxs[1:]
	wasCached = true

	return ctx, wasCached
}

// caches the memory context structure for similar queries
func (cm *ContextManager) ReturnContext(rootCtx *MemoryContext) {
	rootCtx.mu.Lock()
	rootCtx.active = false
	rootCtx.mu.Unlock()

	cm.mu.Lock()
	cm.ctxCache[rootCtx.contextType] = append(cm.ctxCache[rootCtx.contextType], rootCtx)
	cm.mu.Unlock()
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

func NewMemoryContext(config MemoryContextConfig) *MemoryContext {
	mc := &MemoryContext{
		name:          config.Name,
		parent:        config.Parent,
		contextType:   config.ContextType,
		allocStrategy: config.AllocationStrat,
		pools:         make(map[reflect.Type]Pool),
		stats:         &MemContextStats{},
	}

	return mc
}

// Creates and registers a child context with the same
// context type as the parent.
func (mc *MemoryContext) CreateChild(name string) {
	memCtx := mc.allocate(name)
	mc.RegisterChild(memCtx)
}

func (mc *MemoryContext) allocate(name string) *MemoryContext {
	return NewMemoryContext(MemoryContextConfig{
		Name:        name,
		Parent:      mc,
		ContextType: mc.contextType,
	})
}

// Register a child with custom contex type, not the same
// as its parent
func (mc *MemoryContext) RegisterChild(child *MemoryContext) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.children = append(mc.children, child)
}

// Object Methods

func (mc *MemoryContext) CreatePool(objectType reflect.Type, allocator func() any, cleaner func(any), capacity int) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	poolObj := Pool{
		pool:      make([]any, 0, capacity),
		capacity:  capacity,
		mu:        &sync.Mutex{},
		allocator: allocator,
		cleaner:   cleaner,
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
	if !exists {
		return nil
	}

	obj := poolObj.get()
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

	poolObj.put(obj)
	mm.pools[objectType] = poolObj

	size := unsafe.Sizeof(obj)
	atomic.AddUint64(&mm.stats.totalFreed, uint64(size))
	atomic.AddUint64(&mm.stats.currentUsage, -uint64(size))

	return true
}

func (mm *MemoryContext) GetPool(objectType reflect.Type) *Pool {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	poolObj, exists := mm.pools[objectType]
	if !exists {
		return nil
	}

	return &poolObj
}

// ### Stats
func MemorySnap(rootCtx *MemoryContext) *MemContextStats {
	var s MemContextStats

	rootCtx.mu.RLock()
	defer rootCtx.mu.RUnlock()

	collectStats(rootCtx.stats, &s)

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

	collectStats(ctx.stats, destination)

	children := make([]*MemoryContext, len(ctx.children))
	copy(children, ctx.children)

	ctx.mu.RUnlock()

	for _, child := range children {
		childStats(child, destination)
	}
}

func collectStats(source, destination *MemContextStats) {
	destination.currentUsage += source.currentUsage
	destination.totalAllocated += source.totalAllocated
	destination.totalFreed += source.totalFreed
	destination.peakUsage += source.peakUsage
}

type Pool struct {
	allocator func() any
	cleaner   func(any)
	mu        *sync.Mutex
	pool      []any
	capacity  int
}

// if not more objects are available, the pool will double
// in size.
func (pObj *Pool) get() any {
	pObj.mu.Lock()
	defer pObj.mu.Unlock()

	if len(pObj.pool) == 0 {
		pObj.double()
	}

	obj := pObj.pool[len(pObj.pool)-1]
	pObj.pool = pObj.pool[:len(pObj.pool)-1]

	return obj
}

// if everybody has returned their objects, the pool will
// halve in size.
func (pObj *Pool) put(obj any) {
	pObj.mu.Lock()
	defer pObj.mu.Unlock()

	pObj.cleaner(obj)
	pObj.pool = append(pObj.pool, obj)

	// ## TODO - shrink is adding nil to the pool, and its not a good idea to shrink all the time
	// if len(pObj.pool) == pObj.capacity && pObj.capacity > 2 {
	// 	pObj.shrink()
	// }
}

func (poolObj *Pool) double() {
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

func (poolObj *Pool) shrink() {
	if poolObj.capacity <= 1 {
		return
	}

	newCapacity := poolObj.capacity / 2

	newPool := make([]any, newCapacity)
	newPool = append(newPool, poolObj.pool[:newCapacity]...)

	poolObj.capacity = newCapacity
	poolObj.pool = newPool
}
