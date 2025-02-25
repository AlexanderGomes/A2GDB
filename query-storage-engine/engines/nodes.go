package engines

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/scylladb/go-set/strset"
)

// have a final node that puts everything in order

type Node interface {
	GetOutputChan() chan []*RowV2
}

type TableScanNode struct {
	TableName  string
	Dm         *DiskManagerV2
	OutputChan chan []*RowV2
}

func (tsn TableScanNode) GetOutputChan() chan []*RowV2 {
	return tsn.OutputChan
}

type ProjectionNode struct {
	Set        *strset.Set
	InputChan  chan []*RowV2
	OutputChan chan []*RowV2
}

func (tsn ProjectionNode) GetOutputChan() chan []*RowV2 {
	return tsn.OutputChan
}

type FilterNode struct {
	InnerMap   map[string]interface{}
	RefList    map[string]interface{}
	InputChan  chan []*RowV2
	OutputChan chan []*RowV2
}

func (tsn FilterNode) GetOutputChan() chan []*RowV2 {
	return tsn.OutputChan
}

// The sort node will be sorting data far larger than memory,
// it can't just send batches of sorted data to some channel,
// it would be an incomplete operation
type SortNode struct {
	InnerMap  map[string]interface{}
	InputChan chan []*RowV2
}

func (tsn SortNode) GetOutputChan() chan []*RowV2 {
	return nil
}

type AggregateNode struct {
	InnerMap     map[string]interface{}
	GroupKey     string
	SelectedCols []interface{}
	InputChan    chan []*RowV2
	OutputChan   chan []*RowV2
}

func (tsn AggregateNode) GetOutputChan() chan []*RowV2 {
	return nil
}

func TableScan(tableName string, manager *DiskManagerV2) ([]*RowV2, error) {
	var rows []*RowV2

	tableObj, err := GetTableObj(tableName, manager)
	if err != nil {
		return nil, fmt.Errorf("GetTableObj failed: %w", err)
	}

	directoryMap := tableObj.DirectoryPage.Value
	pages, err := GetTablePagesFromDiskTest(tableObj.DataFile)
	if err != nil {
		return nil, fmt.Errorf("GetTablePagesFromDisk failed: %w", err)
	}

	for _, page := range pages {
		pageId := PageID(page.Header.ID)
		pageObj, ok := directoryMap[pageId]
		if !ok {
			return nil, errors.New("pageObj not found")
		}

		for _, location := range pageObj.PointerArray {
			if location.Free {
				continue
			}

			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			row, err := DecodeRow(rowBytes)
			if err != nil {
				return nil, fmt.Errorf("DecodeRow failed: %w", err)
			}

			rows = append(rows, row)
		}
	}

	return rows, nil
}

func GetColInfo(nodeMap, refList map[string]interface{}) ([]interface{}, string, *strset.Set) {
	var groupKey string

	columns, ok := nodeMap["selected_columns"].([]interface{})
	if !ok {
		columns = nodeMap["fields"].([]interface{})
	}

	set := strset.New() // contains all columns to keep
	for _, column := range columns {
		columnStr := column.(string)

		if strings.Contains(columnStr, "$") {
			mapExpSlice := nodeMap["exprs"].([]interface{})
			opObj := mapExpSlice[1].(map[string]interface{})
			opSlice := opObj["operands"].([]interface{})
			opMap := opSlice[0].(map[string]interface{})
			colCode := opMap["name"].(string)

			groupKey = refList[colCode].(string)
			columnStr = groupKey
		}

		cleanedColumn := strings.ReplaceAll(columnStr, "`", "")
		set.Add(cleanedColumn)
	}

	return columns, groupKey, set
}

func Projection(rows []*RowV2, set *strset.Set) {
	for _, row := range rows {
		for field := range row.Values {
			if !set.Has(field) {
				delete(row.Values, field)
			}
		}
	}
}

func Filter(innerMap, refList map[string]interface{}, rows *[]*RowV2) error {
	conditionObj := innerMap["condition"].(map[string]interface{})
	operation := conditionObj["op"].(map[string]interface{})

	switch kind := operation["kind"]; kind {
	case "GREATER_THAN", "LESS_THAN":
		err := intComparison(conditionObj["operands"], refList, rows, kind.(string))
		if err != nil {
			return fmt.Errorf("intComparison failed: %w", err)
		}
	case "EQUALS":
		err := equals(conditionObj["operands"], refList, rows, kind.(string))
		if err != nil {
			return fmt.Errorf("equals failed: %w", err)
		}
	case "AND":
		err := rangeComparison(conditionObj["operands"], refList, rows, kind.(string))
		if err != nil {
			return fmt.Errorf("rangeComparison failed: %w", err)
		}
	default:
		return fmt.Errorf("kind %s not supported", kind)
	}

	return nil
}

func Sort(innerMap map[string]interface{}, rows *[]*RowV2) {
	column := innerMap["column"].(string)
	direction := innerMap["sortDirection"].(string)

	limitPassed := true
	limit, err := strconv.Atoi(innerMap["limit"].(string))
	if err != nil {
		limitPassed = false
	}

	sort.SliceStable(*rows, func(i, j int) bool {
		valI, errI := strconv.Atoi((*rows)[i].Values[column])
		valJ, errJ := strconv.Atoi((*rows)[j].Values[column])

		if errI != nil || errJ != nil {
			log.Fatalf("Error converting string to int (SliceStable): %s, %s", errI, errJ)
			return false
		}

		if direction == "ASC" {
			return valI < valJ
		} else if direction == "DESC" {
			return valI > valJ
		}

		return false
	})

	if limitPassed {
		*rows = (*rows)[:limit]
	}
}

func Aggregate(innerMap map[string]interface{}, colName string, rows *[]*RowV2, selectedCols []interface{}) (map[string]int, error) {
	var resMap map[string]int
	groupMap := map[string][]*RowV2{}

	customFieldSlice := innerMap["selected_columns"].([]interface{})
	//customField := customFieldSlice[len(customFieldSlice)-1].(string)
	groupByField := customFieldSlice[0].(string)

	for _, row := range *rows {
		groupKey := row.Values[groupByField]
		groupMap[groupKey] = append(groupMap[groupKey], row)
	}

	aggInfoMap := innerMap["aggregates"].(map[string]interface{})
	argsSlice := aggInfoMap["args"].([]interface{})

	functionName := aggInfoMap["function"].(string)

	var argName string
	if functionName != "COUNT" {
		argCode := int(argsSlice[0].(float64))
		argName = selectedCols[argCode].(string)
	}

	var err error
	switch functionName {
	case "COUNT":
		resMap = uniqueCount(groupMap)
	case "MAX":
		resMap, err = maxCount(groupMap, argName)
	case "MIN":
		resMap, err = minCount(groupMap, argName)
	case "AVG":
		resMap, err = avgCount(groupMap, colName)
	case "SUM":
		resMap, err = sumCount(groupMap, colName)
	default:
		err = fmt.Errorf("unsupported type: %s", functionName)
	}

	if err != nil {
		return nil, fmt.Errorf("sql function failed: %w", err)
	}

	return resMap, nil
}
