package queryengine


type ExecutionPlan struct {
	Steps []QueryStep
}

type QueryStep struct {
	Operation string
	index     int
}

func GenerateQueryPlan(parsedQuery *ParsedQuery) ExecutionPlan {
	executionPlan := ExecutionPlan{}
	executionPlan.Steps = make([]QueryStep, 0)

	switch parsedQuery.SQLStatementType {
	case "CREATE TABLE":
		CreateTablePlan(&executionPlan, parsedQuery)
	case "INSERT":
		InsertTablePlan(&executionPlan, parsedQuery)
	case "SELECT":
		SelectTablePlan(&executionPlan, parsedQuery)
	case "DELETE":
		DeletePlan(&executionPlan, parsedQuery)
	case "UPDATE":
		UpdatePlan(&executionPlan, parsedQuery)
	}

	return executionPlan
}

func UpdatePlan(executionPlan *ExecutionPlan, P *ParsedQuery) {
	querySteps := []QueryStep{
		{Operation: "GetTable", index: 0},
		{Operation: "DetermineScan"},
		{Operation: "Update"},
	}

	executionPlan.Steps = append(executionPlan.Steps, querySteps...)
}

func DeletePlan(executionPlan *ExecutionPlan, P *ParsedQuery) {
	querySteps := []QueryStep{
		{Operation: "GetTable", index: 0},
		{Operation: "DetermineScan"},
		{Operation: "DeleteFromTable"},
	}

	executionPlan.Steps = append(executionPlan.Steps, querySteps...)
}

func SelectTablePlan(executionPlan *ExecutionPlan, P *ParsedQuery) {
	filterOperation := determineFilterOperation(P.ColumnsSelected)
	querySteps := []QueryStep{
		{Operation: "GetTable", index: 0},
		{Operation: "DetermineScan"},
		{Operation: filterOperation},
	}


	// more thought here
	if P.Joins != nil {
		querySteps = []QueryStep{
			{Operation: "GetTable", index: 0},
			{Operation: filterOperation, index: 0},
			{Operation: "CollectData"},
			{Operation: "GetTable", index: 1},
			{Operation: filterOperation, index: 1},
			{Operation: "CollectData"},
			{Operation: "JoinQueryTable"},
		}
	}

	if P.GroupBy != "" {
		querySteps = []QueryStep{
			{Operation: "GetTable", index: 0},
			{Operation: "GroupByColumn"},
			{Operation: "GroupByFunction"},
			{Operation: "CollectGroupBy"},
		}
	}

	if P.OrderBy != nil {
		querySteps = append(querySteps, QueryStep{Operation: "OrderBy"})
	}

	executionPlan.Steps = append(executionPlan.Steps, querySteps...)
}

func CreateTablePlan(executionPlan *ExecutionPlan, P *ParsedQuery) {
	querySteps := []QueryStep{
		{Operation: "CreateTable"},
	}

	executionPlan.Steps = append(executionPlan.Steps, querySteps...)
}

func determineFilterOperation(columnsSelected []string) string {
	if len(columnsSelected) > 0 && columnsSelected[0] == "*" {
		return "GetAllColumns"
	}
	return "FilterByColumns"
}

func InsertTablePlan(executionPlan *ExecutionPlan, P *ParsedQuery) {
	querySteps := []QueryStep{
		{Operation: "GetTable", index: 0},
		{Operation: "InsertRows"},
	}

	executionPlan.Steps = append(executionPlan.Steps, querySteps...)
}
