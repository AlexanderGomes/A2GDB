package storage

type TableName string
type Catalog struct {
	Tables map[TableName]TableInfo
}

type TableInfo struct {
	Schema            map[string]ColumnType
	NumOfPages        uint32
	UsedSpace         uint64 // bytes
	FreeSpace         uint64 // bytes
	TupleCountTotal   uint32
	TupleCountPerPage uint32
	TupleAvgSize      uint32
	Histogram         map[Column][]Bucket
	UniqueCount       HyperLogLog // probabilistic
	SkipPage          map[PageID]BloomFilter
}

// placeholder
type HyperLogLog struct {
	B         int
	M         int
	Registers []int
}

type Bucket struct {
	Min   uint32
	Max   uint32
	Count uint32
}

type ColumnType struct {
	IsIndex  bool
	IsUnique bool
	Type     string
}

type Column string

// placeholder
type BloomFilter struct {
	Bitset        []bool
	Size          int
	HashFuncCount int
}
