package engines

type Catalog struct {
	Tables map[string]*TableInfo
}

type Column string
type TableInfo struct {
	Schema     map[string]ColumnType
	NumOfPages uint64
}

type ColumnType struct {
	IsIndex bool
	Type    string
}
