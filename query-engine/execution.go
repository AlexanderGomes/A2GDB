package queryengine

import (
	"disk-db/storage"
	"fmt"
	"hash/fnv"
	"os"
	"strings"
	"text/tabwriter"
)

type Query struct {
	Result  []storage.Row
	Message string
}

type QueryEngine struct {
	DB *storage.BufferPoolManager
}

func (qe *QueryEngine) QueryEntryPoint(sql string) (Query, error) {
	parsedSQL, err := Parser(sql)
	if err != nil {
		return Query{}, err
	}

	queryPlan, err := GenerateQueryPlan(parsedSQL)
	if err != nil {
		return Query{}, err
	}

	result, _ := qe.ExecuteQueryPlan(queryPlan, parsedSQL)

	return result, nil
}

func (qe *QueryEngine) ExecuteQueryPlan(qp ExecutionPlan, P *ParsedQuery) (Query, error) {
	query := Query{}
	var page *storage.Page
	keys := []string{}
	pageIds := []storage.PageID{}

	for _, steps := range qp.Steps {
		switch steps.Operation {
		case "GetTable":
			page = GetTable(P, qe.DB, steps, &keys, &pageIds)
		case "GetAllColumns":
			GetAllColumns(page, &query)
		case "FilterByColumns":
			FilterByColumns(page, &query, P)
		case "InsertRows":
			InsertRows(P, &query, qe.DB, page)
		case "CreateTable":
			CreateTable(P, &query, qe.DB)
		case "JoinQueryTable":
			JoinTables(&query, page, P.Joins[0].Condition)
		}
	}

	FreePages(qe.DB, &pageIds)
	FormatQueryResult(&query, &keys)
	return query, nil
}

func FreePages(bpm *storage.BufferPoolManager, ids *[]storage.PageID) {
	for _, id := range *ids {
		bpm.Unpin(id, true)
	}
}

func FormatQueryResult(query *Query, keys *[]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', tabwriter.AlignRight|tabwriter.Debug)

	for _, column := range *keys {
		fmt.Fprintf(w, "      %s   |", column)
	}
	fmt.Fprintln(w, "")

	for range *keys {
		fmt.Fprintf(w, "--------|--------|")
	}
	fmt.Fprintln(w, "")

	for _, row := range query.Result {
		for _, column := range *keys {
			value := row.Values[column]
			fmt.Fprintf(w, "%s\t|", value)
		}
		fmt.Fprintln(w, "")
	}

	fmt.Println(query.Message)

	w.Flush()
}

func JoinTables(query *Query, page *storage.Page, condition string) {
	comparisonParts := strings.Split(condition, "=")
	leftTableCondition := strings.TrimSpace(comparisonParts[0])
	rightTableCondition := strings.TrimSpace(comparisonParts[1])

	queryRowsMap := make(map[string]storage.Row)
	for _, queryRow := range query.Result {
		queryValue := queryRow.Values[leftTableCondition]
		queryRowsMap[queryValue] = queryRow
	}

	var rowSlice []storage.Row
	for _, pageRow := range page.Rows {
		pageValue := pageRow.Values[rightTableCondition]

		if queryRow, ok := queryRowsMap[pageValue]; ok {
			rowSlice = append(rowSlice, pageRow, queryRow)
		}
	}

	query.Result = rowSlice
}

func FilterByColumns(page *storage.Page, query *Query, P *ParsedQuery) {
	for _, row := range page.Rows {
		filteredRow := storage.Row{Values: make(map[string]string)}

		for _, columnName := range P.ColumnsSelected {
			if value, ok := row.Values[columnName]; ok {
				filteredRow.Values[columnName] = value
			}
		}

		query.Result = append(query.Result, filteredRow)
	}
}

func GetAllColumns(page *storage.Page, query *Query) {
	for _, val := range page.Rows {
		query.Result = append(query.Result, val)
	}
	query.Message = "SUCCESS"
}

func GetTable(parsedQuery *ParsedQuery, bpm *storage.BufferPoolManager, step QueryStep, keys *[]string, ids *[]storage.PageID) *storage.Page {
	pageID, _ := hashTableName(parsedQuery.TableReferences[step.index])
	page, err := bpm.FetchPage(storage.PageID(pageID))
	if err != nil {
		fmt.Println(err)
		return nil
	}

	*ids = append(*ids, page.ID)

	for _, rows := range page.Rows {
		for key := range rows.Values {
			*keys = append(*keys, key)
		}
		break
	}

	return page
}

func InsertRows(parsedQuery *ParsedQuery, query *Query, bpm *storage.BufferPoolManager, page *storage.Page) {
	for _, rowInterface := range parsedQuery.Predicates {
		val := rowInterface.(storage.Row)
		for key, value := range val.Values {
			if key == "ID" || key == "id" {
				page.Rows[value] = val
				break
			}
		}
	}

	query.Message = "ROWS INSERTED"
	bpm.InsertPage(page)
}

func CreateTable(parsedQuery *ParsedQuery, query *Query, bpm *storage.BufferPoolManager) {
	table := parsedQuery.TableReferences[0]
	pageID, _ := hashTableName(table)
	offset := CheckIfPageExists(pageID, bpm)
	if offset != 0 {
		query.Message = "PAGE ALREADY EXISTS"
		return
	}

	CreatePage(pageID, query, bpm, parsedQuery)
}

func CreatePage(pageId uint64, query *Query, bpm *storage.BufferPoolManager, P *ParsedQuery) {
	row := storage.Row{Values: make(map[string]string)}
	page := storage.Page{
		ID:   storage.PageID(pageId),
		Rows: make(map[string]storage.Row),
	}

	for i := 0; i < len(P.ColumnsSelected); i++ {
		colum := P.ColumnsSelected[i]
		types := P.Predicates[i]
		row.Values[colum] = types.(string)
	}

	page.Rows["typeID"] = row

	req := storage.DiskReq{
		Page:      page,
		Operation: "WRITE",
	}

	bpm.DiskScheduler.AddReq(req)
	query.Message = "TABLE CREATED"
}

func CheckIfPageExists(pageID uint64, bpm *storage.BufferPoolManager) int {
	offset := 2
	return int(offset)
}

func hashTableName(tableName string) (uint64, error) {
	hasher := fnv.New64a()
	_, err := hasher.Write([]byte(tableName))
	if err != nil {
		return 0, err
	}

	hashValue := hasher.Sum64()

	fmt.Println(hashValue)
	return hashValue, nil
}
