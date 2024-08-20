package queryengine

import (
	"disk-db/storage"
	"fmt"
	"github.com/xwb1989/sqlparser"
)

type ParsedQuery struct {
	SQLStatementType string
	TableReferences  []string
	ColumnsSelected  []string
	Predicates       []interface{}
	Joins            []Join
}

type Join struct {
	LeftTable  string
	RightTable string
	Condition  string
}

func extractCondition(expr sqlparser.Expr) string {
	switch e := expr.(type) {
	case *sqlparser.ComparisonExpr:
		left := extractCondition(e.Left)
		right := extractCondition(e.Right)
		return left + " " + e.Operator + " " + right
	case *sqlparser.ColName:
		return e.Name.String()
	case *sqlparser.SQLVal:
		return "'" + string(e.Val) + "'" // Handle string values with quotes
	case *sqlparser.ParenExpr:
		return "(" + extractCondition(e.Expr) + ")" // Handle parenthesized expressions
	default:
		return "" // Handle other cases or return an empty string
	}
}

func Parser(query string) (*ParsedQuery, error) {
	stmt, err := sqlparser.Parse(query)
	if err != nil {
		return nil, err
	}

	parsedQuery := &ParsedQuery{}

	switch stmt := stmt.(type) {
	case *sqlparser.Select:
		parsedQuery.SQLStatementType = "SELECT"
		for _, expr := range stmt.SelectExprs {
			col, ok := expr.(*sqlparser.AliasedExpr)
			if ok {
				parsedQuery.ColumnsSelected = append(parsedQuery.ColumnsSelected, col.Expr.(*sqlparser.ColName).Name.String())
			} else {
				parsedQuery.ColumnsSelected = append(parsedQuery.ColumnsSelected, "*")
			}
		}

		fromClause := stmt.From
		if fromClause != nil {
			sqlparser.Walk(func(node sqlparser.SQLNode) (kontinue bool, err error) {
				switch n := node.(type) {
				case *sqlparser.AliasedTableExpr:
					tableName, ok := n.Expr.(sqlparser.TableName)
					if ok {
						parsedQuery.TableReferences = append(parsedQuery.TableReferences, tableName.Name.String())
					}
				case *sqlparser.JoinTableExpr:
					table1, table2 := extractJoinTables(n)

					var condition string
					if binaryExpr, ok := n.Condition.On.(*sqlparser.ComparisonExpr); ok {
						if leftCol, leftOk := binaryExpr.Left.(*sqlparser.ColName); leftOk {
							if rightCol, rightOk := binaryExpr.Right.(*sqlparser.ColName); rightOk {
								if leftCol.Name.Equal(rightCol.Name) {
									condition = leftCol.Name.String() + " = " + rightCol.Name.String()
								}
							}
						}
					}

					join := Join{
						LeftTable:  table1,
						RightTable: table2,
						Condition:  condition,
					}

					parsedQuery.Joins = append(parsedQuery.Joins, join)
				}
				return true, nil
			}, stmt.From)
		}

		if stmt.Where != nil {
			fmt.Println("WHERE CLAUSE")
			sqlparser.Walk(func(node sqlparser.SQLNode) (kontinue bool, err error) {
				if comparisonExpr, ok := node.(*sqlparser.ComparisonExpr); ok {
					left := sqlparser.String(comparisonExpr.Left)
					right := sqlparser.String(comparisonExpr.Right)

					operator := comparisonExpr.Operator

					parsedQuery.Predicates = append(parsedQuery.Predicates, left, operator, right)
				}
				return true, nil
			}, stmt.Where.Expr)
		}
	case *sqlparser.DDL:
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

	case *sqlparser.Insert:
		parsedQuery.SQLStatementType = "INSERT"
		tableName := sqlparser.String(stmt.Table)
		parsedQuery.TableReferences = append(parsedQuery.TableReferences, tableName)

		for _, col := range stmt.Columns {
			parsedQuery.ColumnsSelected = append(parsedQuery.ColumnsSelected, sqlparser.String(col))
		}

		rows, ok := stmt.Rows.(sqlparser.Values)
		if !ok {
			return nil, fmt.Errorf("unexpected type for INSERT INTO values")
		}

		for _, row := range rows {
			currRow := storage.RowV2{Values: make(map[string]string)}
			for i, valExpr := range row {
				key := parsedQuery.ColumnsSelected[i]
				value := sqlparser.String(valExpr)
				currRow.Values[key] = value
			}
			parsedQuery.Predicates = append(parsedQuery.Predicates, currRow)
		}

	case *sqlparser.Delete:
		parsedQuery.SQLStatementType = "DELETE"
		for _, tableExpr := range stmt.TableExprs {
			if aliasedTableExpr, ok := tableExpr.(*sqlparser.AliasedTableExpr); ok {
				tableName, ok := aliasedTableExpr.Expr.(sqlparser.TableName)
				if ok {
					parsedQuery.TableReferences = append(parsedQuery.TableReferences, tableName.Name.String())
				}
			}
		}

		if stmt.Where != nil && stmt.Where.Expr != nil {
			condition := extractCondition(stmt.Where.Expr)
			parsedQuery.Predicates = append(parsedQuery.Predicates, condition)
		}
	default:
		return nil, err
	}

	return parsedQuery, nil
}

func extractJoinTables(join *sqlparser.JoinTableExpr) (string, string) {
	table1 := sqlparser.String(join.LeftExpr)
	table2 := sqlparser.String(join.RightExpr)
	return table1, table2
}
