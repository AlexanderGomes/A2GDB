package storage

type PageID uint64
type Page struct {
	ID       PageID
	TABLE    string
	Rows     map[uint64]Row
	IsDirty  bool
	IsPinned bool
}

type Row struct {
	ID     uint64
	Values map[string]string
}

type Offset int64
type DirectoryPage struct {
	Mapping map[PageID]Offset
}
