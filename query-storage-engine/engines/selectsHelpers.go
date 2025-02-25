package engines

import (
	"errors"
	"fmt"
	"math"
	"strconv"
)

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

func sumCount(groupMap map[string][]*RowV2, colName string) (map[string]int, error) {
	sumMap := map[string]int{}

	for k, v := range groupMap {
		var sum int
		for _, row := range v {
			userValStr := row.Values[colName]
			userValInt, err := strconv.Atoi(userValStr)
			if err != nil {
				return nil, fmt.Errorf("(sumCount) - Parsing str => int failed: %w", err)
			}

			sum += userValInt
		}

		sumMap[k] = sum
	}

	return sumMap, nil
}

func avgCount(groupMap map[string][]*RowV2, colName string) (map[string]int, error) {
	avgMap := map[string]int{}

	for k, v := range groupMap {
		var sum int
		for _, row := range v {
			userValStr := row.Values[colName]
			userValInt, err := strconv.Atoi(userValStr)
			if err != nil {
				return nil, fmt.Errorf("(avgCount) - Parsing str => int failed: %w", err)
			}

			sum += userValInt
		}

		avgMap[k] = sum / len(v)
	}

	return avgMap, nil
}

func minCount(groupMap map[string][]*RowV2, field string) (map[string]int, error) {
	minMap := map[string]int{}

	for k, v := range groupMap {
		minAge := math.MaxInt64
		for _, row := range v {
			userValStr := row.Values[field]
			userValInt, err := strconv.Atoi(userValStr)
			if err != nil {
				return nil, fmt.Errorf("(minCount) - Parsing str => int failed: %w", err)
			}
			if userValInt < minAge {
				minAge = userValInt
			}
		}
		minMap[k] = minAge
	}

	return minMap, nil
}

func maxCount(groupMap map[string][]*RowV2, field string) (map[string]int, error) {
	minMap := map[string]int{}

	for k, v := range groupMap {
		var maxAge int
		for _, row := range v {
			ageStr := row.Values[field]
			ageInt, err := strconv.Atoi(ageStr)
			if err != nil {
				return nil, fmt.Errorf("(maxCount) - Parsing str => int failed: %w", err)
			}

			if ageInt > maxAge {
				maxAge = ageInt
			}
		}
		minMap[k] = maxAge
	}

	return minMap, nil
}

func uniqueCount(groupMap map[string][]*RowV2) map[string]int {
	countMap := map[string]int{}

	for k, v := range groupMap {
		countMap[k] = len(v)
	}

	return countMap
}

func equals(conditionObj interface{}, reflist map[string]interface{}, rows *[]*RowV2, kind string) error {
	maps := conditionObj.([]interface{})

	typeObj := maps[1].(map[string]interface{})
	typeMap := typeObj["type"].(map[string]interface{})
	typeName := typeMap["type"].(string)

	switch typeName {
	case "INTEGER", "BIGINT":
		err := intComparison(conditionObj, reflist, rows, kind)
		if err != nil {
			return fmt.Errorf("intComparison failed: %w", err)
		}
	case "VARCHAR":
		err := charComparison(maps, reflist, rows)
		if err != nil {
			return fmt.Errorf("charComparison failed: %w", err)
		}
	case "DECIMAL":
		err := decimalComparison(maps, reflist, rows)
		if err != nil {
			return fmt.Errorf("decimalComparison failed: %w", err)
		}
	}

	return nil
}

func decimalComparison(maps []interface{}, reflist map[string]interface{}, rows *[]*RowV2) error {
	var filteredRows []*RowV2

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
			return errors.New("row value not present")
		}

		if fieldVal == operandVal {
			filteredRows = append(filteredRows, row)
		}
	}

	*rows = filteredRows
	return nil
}

func charComparison(maps []interface{}, reflist map[string]interface{}, rows *[]*RowV2) error {
	var filteredRows []*RowV2

	colNameMap := maps[0].(map[string]interface{})
	colNameCode := colNameMap["name"].(string)
	colName := reflist[colNameCode].(string)

	colComparisonMap := maps[1].(map[string]interface{})
	colComparisonVal := colComparisonMap["literal"].(string)

	for _, row := range *rows {
		fieldVal, ok := row.Values[colName]
		if !ok {
			return errors.New("row value not present")
		}

		if fieldVal == colComparisonVal {
			filteredRows = append(filteredRows, row)
		}

	}

	*rows = filteredRows

	return nil
}

func intComparison(conditionObj interface{}, reflist map[string]interface{}, rows *[]*RowV2, kind string) error {
	var filteredRows []*RowV2
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
			return errors.New("row Value not present")
		}

		parsedUserVal, err := strconv.ParseInt(fieldVal, 10, 64)
		if err != nil {
			return fmt.Errorf("parsing Int Failed: %w", err)
		}

		matchCondition := compare(parsedUserVal, comparisonVal, kind, nil)
		if matchCondition {
			filteredRows = append(filteredRows, row)
		}
	}

	*rows = filteredRows
	return nil
}

func rangeComparison(conditionObj interface{}, reflist map[string]interface{}, rows *[]*RowV2, kind string) error {
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

	var filteredRows []*RowV2
	for _, row := range *rows {
		userValStr, ok := row.Values[columnName]
		if !ok {
			return errors.New("row value not present")
		}

		userValInt, err := strconv.Atoi(userValStr)
		if err != nil {
			return fmt.Errorf("parsing int failed: %w", err)
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
	return nil
}