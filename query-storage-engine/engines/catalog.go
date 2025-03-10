package engines

type Catalog struct {
	Tables map[string]*TableInfo
}

type Column string
type TableInfo struct {
	Schema     map[string]ColumnType
	NumOfPages uint64
	UsedSpace  uint64 // bytes // entire table
}

type ColumnType struct {
	IsIndex bool
	Type    string
}
