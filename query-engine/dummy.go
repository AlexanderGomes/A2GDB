package queryengine

type ParsedQuery struct {
	SQLStatementType string
	TableReferences  []string
	ColumnsSelected  []string
	Predicates       []interface{}
	Joins            *Join
	Where            []string
	SelectFunc       SelectFunc
	GroupBy          string
	OrderBy          *OrderBy
}

type OrderBy struct {
	Column    string
	Operation string
}

type SelectFunc struct {
	FuncName      string
	FuncParameter string
	FuncAlias     string
}

type Join struct {
	LeftTable    string
	RightTable   string
	Condition    Condition
	TableColumns map[string][]string
}

type Condition struct {
	Left   string
	Right  string
	Symbol string
}

type ExecutionPlan struct {
	Steps []QueryStep
}

type QueryStep struct {
	Operation string
	index     int
}


