package storage

type PageID uint64
type Page struct {
	ID       PageID
	TABLE    string
	Rows     map[string]Row
	IsDirty  bool
	IsPinned bool
}

type Row struct {
	Values map[string]string
}

type Offset int64
type DirectoryPage struct {
	Mapping map[PageID]Offset
}
