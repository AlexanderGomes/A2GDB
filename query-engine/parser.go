package queryengine

import (
	"disk-db/storage"
	"fmt"
	"strings"

	"github.com/xwb1989/sqlparser"
)

type ParsedQuery struct {
	SQLStatementType string
	TableReferences  []string
	ColumnsSelected  []string
	Predicates       []interface{}
	Joins            []Join
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
	LeftTable  string
	RightTable string
	Condition  string
}

func Parser(query string) (*ParsedQuery, error) {
	stmt, err := sqlparser.Parse(query)
	if err != nil {
		return nil, err
	}

	parsedQuery := &ParsedQuery{}

	switch stmt := stmt.(type) {
	case *sqlparser.Select:
		processSelect(stmt, parsedQuery)
	case *sqlparser.DDL:
		processDDL(stmt, parsedQuery)
	case *sqlparser.Insert:
		if err := processInsert(stmt, parsedQuery); err != nil {
			return nil, err
		}
	case *sqlparser.Delete:
		processDelete(stmt, parsedQuery)
	case *sqlparser.Update:
		processUpdate(stmt, parsedQuery)
	default:
		return nil, fmt.Errorf("unsupported statement type: %T", stmt)
	}

	return parsedQuery, nil
}

func processSelect(stmt *sqlparser.Select, parsedQuery *ParsedQuery) {
	parsedQuery.SQLStatementType = "SELECT"

	for _, expr := range stmt.SelectExprs {
		if aliasedExpr, ok := expr.(*sqlparser.AliasedExpr); ok {
			switch e := aliasedExpr.Expr.(type) {
			case *sqlparser.FuncExpr:
				funcName := e.Name.String()
				funcParams := make([]string, len(e.Exprs))
				for i, param := range e.Exprs {
					funcParams[i] = sqlparser.String(param)
				}
				parsedQuery.SelectFunc = SelectFunc{
					FuncName:      funcName,
					FuncParameter: strings.Join(funcParams, ", "),
					FuncAlias:     aliasedExpr.As.String(),
				}

			case *sqlparser.ColName:
				parsedQuery.ColumnsSelected = append(parsedQuery.ColumnsSelected, e.Name.String())
			}
		} else {
			parsedQuery.ColumnsSelected = append(parsedQuery.ColumnsSelected, "*")
		}
	}

	if stmt.From != nil {
		sqlparser.Walk(func(node sqlparser.SQLNode) (bool, error) {
			switch n := node.(type) {
			case *sqlparser.AliasedTableExpr:
				if tableName, ok := n.Expr.(sqlparser.TableName); ok {
					parsedQuery.TableReferences = append(parsedQuery.TableReferences, tableName.Name.String())
				}
			case *sqlparser.JoinTableExpr:
				processJoin(n, parsedQuery)
			}
			return true, nil
		}, stmt.From)
	}

	if stmt.Where != nil {
		sqlparser.Walk(func(node sqlparser.SQLNode) (bool, error) {
			if comparisonExpr, ok := node.(*sqlparser.ComparisonExpr); ok {
				parsedQuery.Where = append(parsedQuery.Where,
					sqlparser.String(comparisonExpr.Left),
					sqlparser.String(comparisonExpr.Right))
			}
			return true, nil
		}, stmt.Where.Expr)
	}

	if stmt.GroupBy != nil {
		for _, expr := range stmt.GroupBy {
			col, ok := expr.(*sqlparser.ColName)
			if ok {
				parsedQuery.GroupBy = col.Name.String()
			}
		}
	}

	if stmt.OrderBy != nil {
		for _, expr := range stmt.OrderBy {
			direction := expr.Direction

			var columnName string
			if colName, ok := expr.Expr.(*sqlparser.ColName); ok {
				columnName = colName.Name.String()
			}

			orderBy := OrderBy{
				Operation: direction,
				Column:    columnName,
			}

			parsedQuery.OrderBy = &orderBy
		}
	}
}

func processJoin(join *sqlparser.JoinTableExpr, parsedQuery *ParsedQuery) {
	table1, table2 := extractJoinTables(join)

	var condition string
	if binaryExpr, ok := join.Condition.On.(*sqlparser.ComparisonExpr); ok {
		if leftCol, leftOk := binaryExpr.Left.(*sqlparser.ColName); leftOk {
			if rightCol, rightOk := binaryExpr.Right.(*sqlparser.ColName); rightOk {
				if leftCol.Name.Equal(rightCol.Name) {
					condition = fmt.Sprintf("%s = %s", leftCol.Name.String(), rightCol.Name.String())
				}
			}
		}
	}

	parsedQuery.Joins = append(parsedQuery.Joins, Join{
		LeftTable:  table1,
		RightTable: table2,
		Condition:  condition,
	})
}

func processDDL(stmt *sqlparser.DDL, parsedQuery *ParsedQuery) {
	parsedQuery.SQLStatementType = "CREATE TABLE"
	parsedQuery.TableReferences = append(parsedQuery.TableReferences, sqlparser.String(stmt.NewName))

	for _, col := range stmt.TableSpec.Columns {
		columnName := col.Name.String()
		parsedQuery.ColumnsSelected = append(parsedQuery.ColumnsSelected, columnName)

		columnType := storage.ColumnType{
			Type:    col.Type.SQLType().String(),
			IsIndex: col.Type.KeyOpt == 1,
		}

		parsedQuery.Predicates = append(parsedQuery.Predicates, columnType)
	}
}

func processInsert(stmt *sqlparser.Insert, parsedQuery *ParsedQuery) error {
	parsedQuery.SQLStatementType = "INSERT"
	parsedQuery.TableReferences = append(parsedQuery.TableReferences, sqlparser.String(stmt.Table))

	for _, col := range stmt.Columns {
		parsedQuery.ColumnsSelected = append(parsedQuery.ColumnsSelected, sqlparser.String(col))
	}

	rows, ok := stmt.Rows.(sqlparser.Values)
	if !ok {
		return fmt.Errorf("unexpected type for INSERT INTO values")
	}

	for _, row := range rows {
		currRow := storage.RowV2{Values: make(map[string]string)}
		for i, valExpr := range row {
			key := parsedQuery.ColumnsSelected[i]
			value := sqlparser.String(valExpr)
			currRow.Values[key] = value
		}
		parsedQuery.Predicates = append(parsedQuery.Predicates, &currRow)
	}

	return nil
}

func processDelete(stmt *sqlparser.Delete, parsedQuery *ParsedQuery) {
	parsedQuery.SQLStatementType = "DELETE"
	for _, tableExpr := range stmt.TableExprs {
		if aliasedTableExpr, ok := tableExpr.(*sqlparser.AliasedTableExpr); ok {
			if tableName, ok := aliasedTableExpr.Expr.(sqlparser.TableName); ok {
				parsedQuery.TableReferences = append(parsedQuery.TableReferences, tableName.Name.String())
			}
		}
	}

	if stmt.Where != nil && stmt.Where.Expr != nil {
		sqlparser.Walk(func(node sqlparser.SQLNode) (bool, error) {
			if comparisonExpr, ok := node.(*sqlparser.ComparisonExpr); ok {
				parsedQuery.Where = append(parsedQuery.Where,
					sqlparser.String(comparisonExpr.Left),
					sqlparser.String(comparisonExpr.Right))
			}
			return true, nil
		}, stmt.Where.Expr)
	}
}

func processUpdate(stmt *sqlparser.Update, parsedQuery *ParsedQuery) {
	parsedQuery.SQLStatementType = "UPDATE"
	for _, tableExpr := range stmt.TableExprs {
		if aliasedTableExpr, ok := tableExpr.(*sqlparser.AliasedTableExpr); ok {
			if tableName, ok := aliasedTableExpr.Expr.(sqlparser.TableName); ok {
				parsedQuery.TableReferences = append(parsedQuery.TableReferences, tableName.Name.String())
			}
		}
	}

	for _, expr := range stmt.Exprs {
		parsedQuery.Predicates = append(parsedQuery.Predicates,
			expr.Name.Name.String(),
			sqlparser.String(expr.Expr))
	}

	if stmt.Where != nil && stmt.Where.Expr != nil {
		sqlparser.Walk(func(node sqlparser.SQLNode) (bool, error) {
			if comparisonExpr, ok := node.(*sqlparser.ComparisonExpr); ok {
				parsedQuery.Where = append(parsedQuery.Where,
					sqlparser.String(comparisonExpr.Left),
					sqlparser.String(comparisonExpr.Right))
			}
			return true, nil
		}, stmt.Where.Expr)
	}
}

func extractJoinTables(join *sqlparser.JoinTableExpr) (string, string) {
	return sqlparser.String(join.LeftExpr), sqlparser.String(join.RightExpr)
}
