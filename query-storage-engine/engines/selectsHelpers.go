package engines

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/scylladb/go-set/strset"
)

type LargeComparisons struct {
	Left    int
	Right   int
	UserVal int
}

func compare(a, b int64, operator string, largeComp *LargeComparisons) (bool, error) {
	switch operator {
	case "GREATER_THAN":
		return a > b, nil
	case "LESS_THAN":
		return a < b, nil
	case "EQUALS":
		return a == b, nil
	case "AND":
		return largeComp.UserVal >= largeComp.Left && largeComp.UserVal <= largeComp.Right, nil
	default:
		return false, fmt.Errorf("type not supported: %s", operator)
	}
}

func sumCount(groupMap map[string][]*RowV2, colName string, lm *LockManager) (map[string]int, error) {
	sumMap := map[string]int{}

	for k, v := range groupMap {
		var sum int
		for _, row := range v {

			lm.Lock(row.ID, row, R)
			userValStr := row.Values[colName]
			err := lm.Unlock(row.ID, row, R)
			if err != nil {
				return nil, fmt.Errorf("unlock failed: %w", err)
			}

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

func avgCount(groupMap map[string][]*RowV2, colName string, lm *LockManager) (map[string]int, error) {
	avgMap := map[string]int{}

	for k, v := range groupMap {
		var sum int
		for _, row := range v {
			lm.Lock(row.ID, row, R)
			userValStr := row.Values[colName]
			err := lm.Unlock(row.ID, row, R)
			if err != nil {
				return nil, fmt.Errorf("unlock failed: %w", err)
			}
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

func minCount(groupMap map[string][]*RowV2, field string, lm *LockManager) (map[string]int, error) {
	minMap := map[string]int{}

	for k, v := range groupMap {
		minAge := math.MaxInt64
		for _, row := range v {
			lm.Lock(row.ID, row, R)
			userValStr := row.Values[field]
			err := lm.Unlock(row.ID, row, R)
			if err != nil {
				return nil, fmt.Errorf("unlock failed: %w", err)
			}

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

func maxCount(groupMap map[string][]*RowV2, field string, lm *LockManager) (map[string]int, error) {
	minMap := map[string]int{}

	for k, v := range groupMap {
		var maxAge int
		for _, row := range v {
			lm.Lock(row.ID, row, R)
			ageStr := row.Values[field]
			err := lm.Unlock(row.ID, row, R)
			if err != nil {
				return nil, fmt.Errorf("unlock failed: %w", err)
			}

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

func equals(outerCtx, innerCtx context.Context, lm *LockManager, conditionObj interface{}, reflist map[string]interface{}, kind string, inputChan, outputChan chan []*RowV2) error {
	maps := conditionObj.([]interface{})

	typeObj := maps[1].(map[string]interface{})
	typeMap := typeObj["type"].(map[string]interface{})
	typeName := typeMap["type"].(string)

	switch typeName {
	case "INTEGER", "BIGINT":
		err := intComparison(outerCtx, innerCtx, lm, conditionObj, reflist, kind, inputChan, outputChan)
		if err != nil {
			return fmt.Errorf("intComparison failed: %w", err)
		}
	case "VARCHAR":
		err := charComparison(outerCtx, innerCtx, lm, maps, reflist, inputChan, outputChan)
		if err != nil {
			return fmt.Errorf("charComparison failed: %w", err)
		}
	case "DECIMAL":
		err := decimalComparison(outerCtx, innerCtx, lm, maps, reflist, inputChan, outputChan)
		if err != nil {
			return fmt.Errorf("decimalComparison failed: %w", err)
		}
	}

	return nil
}

func decimalComparison(outerCtx, innerCtx context.Context, lm *LockManager, maps []interface{}, reflist map[string]interface{}, inputChan, outputChan chan []*RowV2) error {
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

	var matchedRows []*RowV2
	for {
		select {
		case <-outerCtx.Done():
			return outerCtx.Err()
		case <-innerCtx.Done():
			return innerCtx.Err()
		case rows, ok := <-inputChan:
			if !ok {
				if len(matchedRows) > 0 {
					outputChan <- matchedRows
				}
				return nil
			}

			for _, row := range rows {

				lm.Lock(row.ID, row, R)
				fieldVal, ok := row.Values[colName]
				if !ok {
					return errors.New("row value not present")
				}
				err := lm.Unlock(row.ID, row, R)
				if err != nil {
					return fmt.Errorf("unlock failed: %w", err)
				}

				if fieldVal == operandVal {
					matchedRows = append(matchedRows, row)
				}

				if len(matchedRows) >= BATCH_THRESHOLD {
					outputChan <- matchedRows
					matchedRows = []*RowV2{}
				}
			}
		}
	}
}

func charComparison(outerCtx, innerCtx context.Context, lm *LockManager, maps []interface{}, reflist map[string]interface{}, inputChan, outputChan chan []*RowV2) error {
	colNameMap := maps[0].(map[string]interface{})
	colNameCode := colNameMap["name"].(string)
	colName := reflist[colNameCode].(string)

	colComparisonMap := maps[1].(map[string]interface{})
	colComparisonVal := colComparisonMap["literal"].(string)

	var matchedRows []*RowV2
	for {
		select {
		case <-outerCtx.Done():
			return outerCtx.Err()
		case <-innerCtx.Done():
			return innerCtx.Err()
		case rows, ok := <-inputChan:
			if !ok {
				if len(matchedRows) > 0 {
					outputChan <- matchedRows
				}
				return nil
			}

			for _, row := range rows {
				lm.Lock(row.ID, row, R)
				fieldVal, ok := row.Values[colName]
				if !ok {
					return errors.New("row value not present")
				}

				err := lm.Unlock(row.ID, row, R)
				if err != nil {
					return fmt.Errorf("unlock failed: %w", err)
				}

				if fieldVal == colComparisonVal {
					matchedRows = append(matchedRows, row)
				}

				if len(matchedRows) >= BATCH_THRESHOLD {
					outputChan <- matchedRows
					matchedRows = []*RowV2{}
				}
			}
		}
	}
}

func intComparison(outerCtx, innerCtx context.Context, lm *LockManager, conditionObj interface{}, reflist map[string]interface{}, kind string, inputChan, outputChan chan []*RowV2) error {
	maps := conditionObj.([]interface{})

	colObjMap := maps[0].(map[string]interface{})
	colNameMapSlice := colObjMap["operands"].([]interface{})
	colNameMap := colNameMapSlice[0].(map[string]interface{})

	valMap := maps[1].(map[string]interface{})

	colCode := colNameMap["name"].(string)
	comparisonVal := int64(valMap["literal"].(float64))
	colName := reflist[colCode].(string)

	var matchedRows []*RowV2
	for {
		select {
		case <-outerCtx.Done():
			return outerCtx.Err()
		case <-innerCtx.Done():
			return innerCtx.Err()
		case rows, ok := <-inputChan:
			if !ok {
				if len(matchedRows) > 0 {
					outputChan <- matchedRows
				}
				return nil
			}

			for _, row := range rows {
				lm.Lock(row.ID, row, R)
				fieldVal, ok := row.Values[colName]
				if !ok {
					return errors.New("row Value not present")
				}
				err := lm.Unlock(row.ID, row, R)
				if err != nil {
					fmt.Println(row)
					return fmt.Errorf("Unlock failed: %w", err)
				}

				parsedUserVal, err := strconv.ParseInt(fieldVal, 10, 64)
				if err != nil {
					return fmt.Errorf("parsing Int Failed: %w", err)
				}

				conditionMatch, err := compare(parsedUserVal, comparisonVal, kind, nil)
				if err != nil {
					return fmt.Errorf("compare failed: %w", err)
				}
				if conditionMatch {
					matchedRows = append(matchedRows, row)
				}

				if len(matchedRows) >= BATCH_THRESHOLD {
					outputChan <- matchedRows
					matchedRows = []*RowV2{}
				}
			}
		}
	}
}

func rangeComparison(outerCtx, innerCtx context.Context, lm *LockManager, conditionObj interface{}, reflist map[string]interface{}, kind string, inputChan, outputChan chan []*RowV2) error {
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

	var matchedRows []*RowV2
	for {
		select {
		case <-outerCtx.Done():
			return outerCtx.Err()
		case <-innerCtx.Done():
			return innerCtx.Err()
		case rows, ok := <-inputChan:
			if !ok {
				if len(matchedRows) > 0 {
					outputChan <- matchedRows
				}
				return nil
			}

			for _, row := range rows {

				lm.Lock(row.ID, row, R)
				userValStr, ok := row.Values[columnName]
				if !ok {
					return errors.New("row value not present")
				}
				err := lm.Unlock(row.ID, row, R)
				if err != nil {
					return fmt.Errorf("unlock failed: %w", err)
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

				matched, err := compare(0, 0, kind, &largeComp)
				if err != nil {
					return fmt.Errorf("compare failed: %w", err)
				}
				if matched {
					matchedRows = append(matchedRows, row)
				}

				if len(matchedRows) >= BATCH_THRESHOLD {
					outputChan <- matchedRows
					matchedRows = []*RowV2{}
				}
			}
		}
	}
}

func RowCollector(outerCtx, innerCtx context.Context, pageChan chan *PageV2, outputChan chan []*RowV2, tableObj *TableObj) error {
	var rows []*RowV2

	for {
		select {
		case <-outerCtx.Done():
			return outerCtx.Err()
		case <-innerCtx.Done():
			return innerCtx.Err()
		case page, ok := <-pageChan:
			if !ok {
				if len(rows) > 0 {
					outputChan <- rows
				}

				return nil
			}

			tableObj.DirectoryPage.Mu.RLock()
			pageObj, found := tableObj.DirectoryPage.Value[PageID(page.Header.ID)]
			if !found {
				return fmt.Errorf("pageObj not found, pageId: %d", page.Header.ID)
			}
			tableObj.DirectoryPage.Mu.RUnlock()

			pageObj.Mu.RLock()
			for _, location := range pageObj.PointerArray {
				if location.Free {
					continue
				}

				// TODO - possible change
				rowBytes := page.Data[location.Offset : location.Offset+location.Length]
				buf := bytes.NewReader(rowBytes)
				var row RowV2

				DecodeRow(&row, buf)

				rows = append(rows, &row)
				if len(rows) >= BATCH_THRESHOLD {
					outputChan <- rows
					rows = []*RowV2{}
				}
			}
			pageObj.Mu.RUnlock()
		}
	}
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

func Projection(outerCtx, innerCtx context.Context, lm *LockManager, inputChan chan []*RowV2, outputChan chan []*RowV2, set *strset.Set) error {
	for {
		select {
		case <-outerCtx.Done():
			return outerCtx.Err()
		case <-innerCtx.Done():
			return innerCtx.Err()
		case rows, ok := <-inputChan:
			if !ok {
				return nil
			}

			for _, row := range rows {
				lm.Lock(row.ID, row, W)
				for field := range row.Values {
					if !set.Has(field) {
						delete(row.Values, field)
					}
				}
				err := lm.Unlock(row.ID, row, W)
				if err != nil {
					return fmt.Errorf("unlock failed: %w", err)
				}
			}

			outputChan <- rows
		}
	}
}

func convertToRow(resMap map[string]int) *RowV2 {
	row := RowV2{Values: make(map[string]string)}

	for k, v := range resMap {
		row.Values[k] = fmt.Sprintf("%d", v)
	}

	return &row
}

func Filter(outerCtx, innerCtx context.Context, lm *LockManager, innerMap, refList map[string]interface{}, inputChan, outputChan chan []*RowV2) error {
	conditionObj := innerMap["condition"].(map[string]interface{})
	operation := conditionObj["op"].(map[string]interface{})

	switch kind := operation["kind"]; kind {
	case "GREATER_THAN", "LESS_THAN":
		err := intComparison(outerCtx, innerCtx, lm, conditionObj["operands"], refList, kind.(string), inputChan, outputChan)
		if err != nil {
			return fmt.Errorf("intComparison failed: %w", err)
		}
	case "EQUALS":
		err := equals(outerCtx, innerCtx, lm, conditionObj["operands"], refList, kind.(string), inputChan, outputChan)
		if err != nil {
			return fmt.Errorf("equals failed: %w", err)
		}
	case "AND":
		err := rangeComparison(outerCtx, innerCtx, lm, conditionObj["operands"], refList, kind.(string), inputChan, outputChan)
		if err != nil {
			return fmt.Errorf("rangeComparison failed: %w", err)
		}
	default:
		return fmt.Errorf("kind %s not supported", kind)
	}

	return nil
}

func Sort(ctx context.Context, lm *LockManager, innerMap map[string]interface{}, rows *[]*RowV2, outputChan chan []*RowV2) error {
	column := innerMap["column"].(string)
	direction := innerMap["sortDirection"].(string)

	limitPassed := true
	limit, err := strconv.Atoi(innerMap["limit"].(string))
	if err != nil {
		limitPassed = false
	}

	sort.SliceStable(*rows, func(i, j int) bool {
		select {
		case <-ctx.Done():
			return false
		default:
			rowI := (*rows)[i]
			rowJ := (*rows)[j]

			lm.Lock(rowI.ID, rowI, R)
			valI, errI := strconv.Atoi(rowI.Values[column])
			err = lm.Unlock(rowI.ID, rowI, R)
			if err != nil {
				return false
			}

			lm.Lock(rowJ.ID, rowJ, R)
			valJ, errJ := strconv.Atoi(rowJ.Values[column])
			err := lm.Unlock(rowJ.ID, rowJ, R)
			if err != nil {
				return false
			}

			if errI != nil || errJ != nil {
				log.Fatalf("Error converting string to int (SliceStable): %s, %s", errI, errJ)
				return false
			}

			if direction == "ASC" {
				return valI < valJ
			} else if direction == "DESC" {
				return valI > valJ
			}
		}
		return false
	})

	if limitPassed {
		*rows = (*rows)[:limit]
	}

	outputChan <- *rows
	return nil
}

func Aggregate(ctx context.Context, lm *LockManager, innerMap map[string]interface{}, colName string, rows *[]*RowV2, selectedCols []interface{}, outputChan chan []*RowV2) error {
	var resMap map[string]int
	groupMap := map[string][]*RowV2{}

	customFieldSlice := innerMap["selected_columns"].([]interface{})
	//customField := customFieldSlice[len(customFieldSlice)-1].(string)
	groupByField := customFieldSlice[0].(string)

	for _, row := range *rows {

		lm.Lock(row.ID, row, R)
		groupKey := row.Values[groupByField]
		err := lm.Unlock(row.ID, row, R)
		if err != nil {
			return fmt.Errorf("unlock failed: %w", err)
		}

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
		resMap, err = maxCount(groupMap, argName, lm)
	case "MIN":
		resMap, err = minCount(groupMap, argName, lm)
	case "AVG":
		resMap, err = avgCount(groupMap, colName, lm)
	case "SUM":
		resMap, err = sumCount(groupMap, colName, lm)
	default:
		err = fmt.Errorf("unsupported type: %s", functionName)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		row := convertToRow(resMap)
		outputChan <- []*RowV2{row}
	}

	if err != nil {
		return fmt.Errorf("sql function failed: %w", err)
	}

	return nil
}

func ComputeNodes(plan map[string]interface{}, qe *QueryEngine) ([]Node, error) {
	var selectedCols []interface{}
	var groupKey string
	var physicalNodes []Node
	var set *strset.Set

	logicalNodes := plan["rels"].([]interface{})
	referenceList := plan["refList"].(map[string]interface{})
	for _, node := range logicalNodes {
		nodeInnerMap := node.(map[string]interface{})

		switch nodeOperation := nodeInnerMap["relOp"]; nodeOperation {
		case "LogicalTableScan":
			tableName := nodeInnerMap["table"].([]interface{})[0].(string)
			scanNode := TableScanNode{
				Type:       "TableScanNode",
				TableName:  tableName,
				Dm:         qe.BufferPoolManager,
				OutputChan: make(chan []*RowV2, 10),
			}

			physicalNodes = append(physicalNodes, scanNode)
		case "LogicalProject":
			selectedCols, groupKey, set = GetColInfo(nodeInnerMap, referenceList)
			projectNode := ProjectionNode{
				Type:       "ProjectionNode",
				Lm:         qe.Lm,
				Set:        set,
				InputChan:  physicalNodes[len(physicalNodes)-1].GetOutputChan(),
				OutputChan: make(chan []*RowV2, 10),
			}
			physicalNodes = append(physicalNodes, projectNode)
		case "LogicalFilter":
			filterNode := FilterNode{
				Type:       "FilterNode",
				Lm:         qe.Lm,
				InnerMap:   nodeInnerMap,
				RefList:    referenceList,
				InputChan:  physicalNodes[len(physicalNodes)-1].GetOutputChan(),
				OutputChan: make(chan []*RowV2, 10),
			}

			physicalNodes = append(physicalNodes, filterNode)
		case "LogicalSort":
			sortNode := SortNode{
				Type:       "SortNode",
				Lm:         qe.Lm,
				InnerMap:   nodeInnerMap,
				InputChan:  physicalNodes[len(physicalNodes)-1].GetOutputChan(),
				OutputChan: make(chan []*RowV2, 10),
			}

			physicalNodes = append(physicalNodes, sortNode)
		case "LogicalAggregate":
			aggregateNode := AggregateNode{
				Type:         "AggregateNode",
				Lm:           qe.Lm,
				InnerMap:     nodeInnerMap,
				GroupKey:     groupKey,
				SelectedCols: selectedCols,
				InputChan:    physicalNodes[len(physicalNodes)-1].GetOutputChan(),
				OutputChan:   make(chan []*RowV2, 10),
			}

			physicalNodes = append(physicalNodes, aggregateNode)
		default:
			return []Node{}, fmt.Errorf("unsupported type: %s", nodeOperation)
		}
	}

	collector := CollectorNode{
		Type:      "CollectorNode",
		InputChan: physicalNodes[len(physicalNodes)-1].GetOutputChan(),
		Rows:      &[]*RowV2{},
	}

	physicalNodes = append(physicalNodes, collector)

	return physicalNodes, nil
}
