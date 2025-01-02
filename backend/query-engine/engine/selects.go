package engine


import (
	"a2gdb/storage-engine/storage"
	"github.com/scylladb/go-set/strset"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
)

func groupBy(innerMap map[string]interface{}, colName string, rows *[]*storage.RowV2, selectedCols []interface{}) map[string]int {
	var resMap map[string]int
	groupMap := map[string][]*storage.RowV2{}

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

	switch functionName {
	case "COUNT":
		resMap = uniqueCount(groupMap)
	case "MAX":
		resMap = maxCount(groupMap, argName)
	case "MIN":
		resMap = minCount(groupMap, argName)
	case "AVG":
		resMap = avgCount(groupMap, colName)
	case "SUM":
		resMap = sumCount(groupMap, colName)
	default:
		log.Fatalf("Unsupported type: %s", functionName)
	}

	return resMap
}

func sumCount(groupMap map[string][]*storage.RowV2, colName string) map[string]int {
	sumMap := map[string]int{}

	for k, v := range groupMap {
		var sum int
		for _, row := range v {
			userValStr := row.Values[colName]
			userValInt, err := strconv.Atoi(userValStr)
			if err != nil {
				log.Fatalf("Error converting string to int: %s", err)
			}

			sum += userValInt
		}

		sumMap[k] = sum
	}

	return sumMap
}

func avgCount(groupMap map[string][]*storage.RowV2, colName string) map[string]int {
	avgMap := map[string]int{}

	for k, v := range groupMap {
		var sum int
		for _, row := range v {
			userValStr := row.Values[colName]
			userValInt, err := strconv.Atoi(userValStr)
			if err != nil {
				log.Fatalf("Error converting string to int: %s", err)
			}

			sum += userValInt
		}

		avgMap[k] = sum / len(v)
	}

	return avgMap
}

func minCount(groupMap map[string][]*storage.RowV2, field string) map[string]int {
	maxtMap := map[string]int{}

	for k, v := range groupMap {
		minAge := math.MaxInt64
		for _, row := range v {
			userValStr := row.Values[field]
			userValInt, err := strconv.Atoi(userValStr)
			if err != nil {
				log.Fatalf("Error converting string to int: %s", err)
			}
			if userValInt < minAge {
				minAge = userValInt
			}
		}
		maxtMap[k] = minAge
	}

	return maxtMap
}

func maxCount(groupMap map[string][]*storage.RowV2, field string) map[string]int {
	minMap := map[string]int{}

	for k, v := range groupMap {
		var maxAge int
		for _, row := range v {
			ageStr := row.Values[field]
			ageInt, err := strconv.Atoi(ageStr)
			if err != nil {
				log.Fatalf("Error converting string to int: %s", err)
			}

			if ageInt > maxAge {
				maxAge = ageInt
			}
		}
		minMap[k] = maxAge
	}

	return minMap
}

func uniqueCount(groupMap map[string][]*storage.RowV2) map[string]int {
	countMap := map[string]int{}

	for k, v := range groupMap {
		countMap[k] = len(v)
	}

	return countMap
}

func sortAscDesc(innerMap map[string]interface{}, rows *[]*storage.RowV2) {
	column := innerMap["column"].(string)
	direction := innerMap["sortDirection"].(string)

	limitPassed := true
	limit, err := strconv.Atoi(innerMap["limit"].(string))
	if err != nil {
		log.Println("No limit passed")
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
		return
	}
}

func filterByColumn(innerMap, refList map[string]interface{}, rows *[]*storage.RowV2) {
	conditionObj := innerMap["condition"].(map[string]interface{})
	operation := conditionObj["op"].(map[string]interface{})

	switch kind := operation["kind"]; kind {
	case "GREATER_THAN", "LESS_THAN":
		intComparison(conditionObj["operands"], refList, rows, kind.(string))
	case "EQUALS":
		equals(conditionObj["operands"], refList, rows, kind.(string))
	case "AND":
		rangeComparison(conditionObj["operands"], refList, rows, kind.(string))
	default:
		log.Fatalf("kind %s not supported", kind)
	}
}

type LargeComparisons struct {
	Left    int
	Right   int
	UserVal int
}

func compare(a, b int64, operator string, largeComp *LargeComparisons) bool {
	switch operator {
	case "GREATER_THAN":
		return a > b
	case "LESS_THAN":
		return a < b
	case "EQUALS":
		return a == b
	case "AND":
		return largeComp.UserVal >= largeComp.Left && largeComp.UserVal <= largeComp.Right
	default:
		return false
	}
}

func rangeComparison(conditionObj interface{}, reflist map[string]interface{}, rows *[]*storage.RowV2, kind string) {
	maps := conditionObj.([]interface{})

	leftObjOp := maps[0].(map[string]interface{})
	leftObj := leftObjOp["operands"].([]interface{})
	leftNameMaps := leftObj[0].(map[string]interface{})
	leftNameSliceMap := leftNameMaps["operands"].([]interface{})
	leftNameMap := leftNameSliceMap[0].(map[string]interface{})
	colCode := leftNameMap["name"].(string)
	leftValMap := leftObj[1].(map[string]interface{})

	rightObjOp := maps[1].(map[string]interface{})
	rightValSlice := rightObjOp["operands"].([]interface{})
	rightValMap := rightValSlice[1].(map[string]interface{})

	columnName := reflist[colCode].(string)
	leftVal := int(leftValMap["literal"].(float64))
	rightVal := int(rightValMap["literal"].(float64))

	var filteredRows []*storage.RowV2
	for _, row := range *rows {
		userValStr, ok := row.Values[columnName]
		if !ok {
			log.Fatalf("field: %s not present in row: %d", columnName, row.ID)
		}

		userValInt, err := strconv.Atoi(userValStr)
		if err != nil {
			log.Fatalf("Error converting string to int: %s", err)
		}

		largeComp := LargeComparisons{
			Left:    leftVal,
			Right:   rightVal,
			UserVal: userValInt,
		}

		matched := compare(0, 0, kind, &largeComp)
		if matched {
			filteredRows = append(filteredRows, row)
		}
	}

	*rows = filteredRows
}

func equals(conditionObj interface{}, reflist map[string]interface{}, rows *[]*storage.RowV2, kind string) {
	maps := conditionObj.([]interface{})

	typeObj := maps[1].(map[string]interface{})
	typeMap := typeObj["type"].(map[string]interface{})
	typeName := typeMap["type"].(string)

	switch typeName {
	case "INTEGER", "BIGINT":
		intComparison(conditionObj, reflist, rows, kind)
	case "VARCHAR":
		charComparison(maps, reflist, rows)
	case "DECIMAL":
		decimalComparison(maps, reflist, rows)
	}

}

func decimalComparison(maps []interface{}, reflist map[string]interface{}, rows *[]*storage.RowV2) {
	var filteredRows []*storage.RowV2

	colNameObj := maps[0].(map[string]interface{})
	colNameMapSlice := colNameObj["operands"].([]interface{})
	colNameMap := colNameMapSlice[0].(map[string]interface{})
	colNameCode := colNameMap["name"].(string)

	colValObj := maps[1].(map[string]interface{})
	colValMapSlice := colValObj["operands"].([]interface{})
	colValMap := colValMapSlice[0].(map[string]interface{})
	operandsSlice := colValMap["operands"].([]interface{})
	operandMap := operandsSlice[0].(map[string]interface{})

	operandVal := operandMap["literal"].(string)
	colName := reflist[colNameCode].(string)

	for _, row := range *rows {
		fieldVal, ok := row.Values[colName]
		if !ok {
			log.Fatalf("field: %s not present in row: %d", colName, row.ID)
		}

		if fieldVal == operandVal {
			filteredRows = append(filteredRows, row)
		}
	}

	*rows = filteredRows
}

func charComparison(maps []interface{}, reflist map[string]interface{}, rows *[]*storage.RowV2) {
	var filteredRows []*storage.RowV2

	colNameMap := maps[0].(map[string]interface{})
	colNameCode := colNameMap["name"].(string)
	colName := reflist[colNameCode].(string)

	colComparisonMap := maps[1].(map[string]interface{})
	colComparisonVal := colComparisonMap["literal"].(string)

	for _, row := range *rows {
		fieldVal, ok := row.Values[colName]
		if !ok {
			log.Fatalf("field: %s not present in row: %d", colName, row.ID)
		}

		if fieldVal == colComparisonVal {
			filteredRows = append(filteredRows, row)
		}

	}

	*rows = filteredRows
}

func intComparison(conditionObj interface{}, reflist map[string]interface{}, rows *[]*storage.RowV2, kind string) {
	var filteredRows []*storage.RowV2
	maps := conditionObj.([]interface{})

	colObjMap := maps[0].(map[string]interface{})
	colNameMapSlice := colObjMap["operands"].([]interface{})
	colNameMap := colNameMapSlice[0].(map[string]interface{})

	valMap := maps[1].(map[string]interface{})

	colCode := colNameMap["name"].(string)
	comparisonVal := int64(valMap["literal"].(float64))
	colName := reflist[colCode].(string)

	for _, row := range *rows {
		fieldVal, ok := row.Values[colName]
		if !ok {
			log.Fatalf("field: %s not present in row: %d", colName, row.ID)
		}

		parsedUserVal, err := strconv.ParseInt(fieldVal, 10, 64)
		if err != nil {
			log.Fatalf("failed parsing %s, for row: %d", fieldVal, row.ID)
		}

		matchCondition := compare(parsedUserVal, comparisonVal, kind, nil)
		if matchCondition {
			filteredRows = append(filteredRows, row)
		}
	}

	*rows = filteredRows
}

func columnSelect(nodeMap, refList map[string]interface{}, rows []*storage.RowV2) ([]interface{}, string) {
	var colName string

	columns, ok := nodeMap["selected_columns"].([]interface{})
	if !ok {
		columns = nodeMap["fields"].([]interface{})
	}

	set := strset.New()
	for _, column := range columns {
		columnStr := column.(string)

		if strings.Contains(columnStr, "$") {
			mapExpSlice := nodeMap["exprs"].([]interface{})
			opObj := mapExpSlice[1].(map[string]interface{})
			opSlice := opObj["operands"].([]interface{})
			opMap := opSlice[0].(map[string]interface{})
			colCode := opMap["name"].(string)

			colName = refList[colCode].(string)
			columnStr = colName
		}

		cleanedColumn := strings.ReplaceAll(columnStr, "`", "")
		set.Add(cleanedColumn)
	}

	for _, row := range rows {
		for field := range row.Values {
			if !set.Has(field) {
				delete(row.Values, field)
			}
		}
	}

	return columns, colName
}
