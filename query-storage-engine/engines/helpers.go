package engines

import (
	"a2gdb/logger"
	"a2gdb/utils"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
)

func prepareRows(plan map[string]interface{}, selectedCols []interface{}, primary, tableName, txID string, wal *WalManager, transactionOff bool) (uint16, [][]byte, error) {
	var bytesNeeded uint16
	var encodedRows [][]byte

	interfaceRows := plan["rows"].([]interface{})

	for _, row := range interfaceRows {
		newRow := RowV2{
			ID:     GenerateRandomID(),
			Values: make(map[string]string),
		}

		//#Add row values
		newRow.Values[primary] = strconv.FormatUint(newRow.ID, 10)
		for i, rowVal := range row.([]any) {
			strRowVal := strings.ReplaceAll(rowVal.(string), "'", "")
			strRowCol := selectedCols[i].(string)

			newRow.Values[strRowCol] = strRowVal
		}

		buff := BufferAllocator() // ## TODO - POSSIBLE CHANGE
		encodedRow, err := EncodeRow(&newRow, buff.(*bytes.Buffer))
		if err != nil {
			return 0, nil, fmt.Errorf("encodeRow failed: %w", err)
		}

		if !transactionOff {
			err = wal.Log(txID, LogTypeInsert, tableName, newRow.ID, nil, encodedRow)
			if err != nil {
				return 0, nil, fmt.Errorf("wal.log failed: %w", err)
			}
		}

		bytesNeeded += uint16(len(encodedRow))
		encodedRows = append(encodedRows, encodedRow)
	}

	return bytesNeeded, encodedRows, nil
}

func findAndUpdate(bufferM *BufferPoolManager, tableObj *TableObj, tableStats *TableInfo, bytesNeeded uint16, tableName string, encodedRows [][]byte) error {
	page, err := getAvailablePage(bufferM, tableObj, bytesNeeded, tableName) // new page could've been created
	if err != nil {
		return fmt.Errorf("getAvailablePage failed: %w", err)
	}

	newSpace := FreeSpace{
		PageID:     PageID(page.Header.ID),
		FreeMemory: page.Header.UpperPtr - page.Header.LowerPtr, //assuming new page
	}

	tableObj.DirectoryPage.Mu.RLock()
	pageInfoObj, ok := tableObj.DirectoryPage.Value[PageID(page.Header.ID)]
	tableObj.DirectoryPage.Mu.RUnlock()
	if ok {
		pageInfoObj.Mu.RLock()
		newSpace.FreeMemory = pageInfoObj.ExactFreeMem
		pageInfoObj.Mu.RUnlock()
	}

	for _, encodedRow := range encodedRows {
		newSpace.FreeMemory -= uint16(len(encodedRow))
		err := page.AddTuple(encodedRow, "findAndUpdate")
		if err != nil {
			return fmt.Errorf("AddTuple failed: %w", err)
		}
	}

	logger.Log.Info("saving page to disk (created / existing)")
	err = UpdatePageInfo(page, tableObj, tableStats, bufferM.DiskManager, ADDING) // make sure to save possible new page (this is updating even already existing pages)
	if err != nil {
		return fmt.Errorf("UpdatePageInfo failed: %v", page)
	}

	logger.Log.WithFields(logrus.Fields{"newSpace": newSpace}).Info("memSeparationSingle input")
	err = memSeparationSingle(&newSpace, tableObj, tableStats) // safe to do memory separation
	if err != nil {
		return fmt.Errorf("memSeparationSingle failed: %v", page)
	}

	return nil
}

func isPrimary(key string, tableName string, catalog *Catalog) (bool, error) {
	tableInfo, ok := catalog.Tables[tableName]
	if !ok {
		return false, fmt.Errorf("table: %s doesn't exist", tableName)
	}

	columnInfo := tableInfo.Schema[key]

	return columnInfo.IsIndex, nil
}

func checkPresenceGetPrimary(selectedCols []interface{}, tableName string, catalog *Catalog) (string, error) {
	var primary string

	// #check if table exist
	tableInfo, ok := catalog.Tables[tableName]
	if !ok {
		return "", fmt.Errorf("table: %s doesn't exist", tableName)
	}

	// #check if cols exist
	for _, selectedCol := range selectedCols {
		selectedCol := selectedCol.(string)

		_, ok := tableInfo.Schema[selectedCol]
		if !ok {
			return "", fmt.Errorf("column: %s on table: %s doesn't exist", selectedCol, tableName)
		}
	}

	//#get primary
	for column, columnInfo := range tableInfo.Schema {
		if columnInfo.IsIndex {
			primary = column
		}
	}

	if primary == "" {
		return "", fmt.Errorf("primary doesn't exist")
	}

	return primary, nil
}

func processPagesForDeletion(ctx context.Context, lm *LockManager, pages chan *PageV2, updateInfoChan chan *ModifiedInfo, deleteKey, deleteVal, txID string, isPrimary bool, tableObj *TableObj, wal *WalManager, txOff bool) error {
	defer close(updateInfoChan)

	var foundMatch bool
	for page := range pages {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if isPrimary && foundMatch {
			break
		}

		var freeSpacePage *FreeSpace
		var updateInfo ModifiedInfo

		tableObj.DirectoryPage.Mu.RLock()
		pageObj, ok := tableObj.DirectoryPage.Value[PageID(page.Header.ID)]
		if !ok {
			return errors.New("pageObj missing")
		}
		tableObj.DirectoryPage.Mu.RUnlock()

		pageObj.Mu.Lock()

		for i := range pageObj.PointerArray {
			location := &pageObj.PointerArray[i]
			if location.Free {
				continue
			}

			rowBytes := page.Data[location.Offset : location.Offset+location.Length]
			var row RowV2
			buf := bytes.NewReader(rowBytes)
			DecodeRow(&row, buf)

			lm.Lock(row.ID, &row, R)
			deleteMatchFound := row.Values[deleteKey] == deleteVal
			err := lm.Unlock(row.ID, &row, R)
			if err != nil {
				return fmt.Errorf("unlock failed: %w", err)
			}

			if deleteMatchFound {
				if freeSpacePage == nil {
					freeSpacePage = &FreeSpace{
						PageID:      PageID(page.Header.ID),
						TempPagePtr: page,
						FreeMemory:  pageObj.ExactFreeMem}
				}

				if !txOff {
					err = wal.Log(txID, LogTypeDelete, tableObj.TableName, row.ID, rowBytes, nil)
					if err != nil {
						return fmt.Errorf("wal.log failed: %w", err)
					}
				}

				freeSpacePage.FreeMemory += location.Length
				location.Free = true

				if isPrimary {
					foundMatch = true
					break
				}
			}
		}

		pageObj.Mu.Unlock()
		if freeSpacePage != nil {
			updateInfo.FreeSpaceMapping = freeSpacePage
			updateInfoChan <- &updateInfo
		}
	}

	return nil
}

type NonAddedRows struct {
	BytesNeeded uint16
	Rows        [][]byte
}

type ModifiedInfo struct {
	FreeSpaceMapping *FreeSpace
	NonAddedRow      *NonAddedRows
}

func processPagesForUpdate(ctx context.Context, accountingCtx *MemoryContext, qe *QueryEngine, lm *LockManager, pageChan chan *PageV2, updateInfoChan chan *ModifiedInfo, updateKey, updateVal, filterKey, filterVal, txID string, tableObj *TableObj, wal *WalManager, txOff bool) error {
	logger.Log.Info("processPagesForUpdate (start)")
	defer close(updateInfoChan)

	tupleCtx, wasCached := qe.CtxManager.GetOrCreateContext(TupleLevel, MemoryContextConfig{Name: "Tuples", ContextType: TupleLevel, AllocationStrat: DefaultAllocation})
	if !wasCached {
		CreateTuplePools(tupleCtx)
	}

	row, reader, buffer, slice := GetTupleObjs(tupleCtx)
	rowpObj, readerpObj, bufferpObj, slicepObj := GetTuplePoolObjs(tupleCtx)

	for page := range pageChan {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		freeSpacePage, updateInfo, nonAddedRows := GetAccountingObjs(accountingCtx)

		pageId := PageID(page.Header.ID)

		directoryPage := tableObj.DirectoryPage

		directoryPage.Mu.RLock()
		pageObj := directoryPage.Value[pageId]
		directoryPage.Mu.RUnlock()

		pageObj.Mu.Lock()
		logger.Log.WithFields(logrus.Fields{"Memlevel": pageObj.Level, "exactFreeMem": pageObj.ExactFreeMem, "offset": pageObj.Offset}).Info("Before Modification (PageObj)")
		for i := range pageObj.PointerArray {
			location := &pageObj.PointerArray[i]

			if location.Free {
				continue
			}

			SliceBytesExpression(slice, page.Data, location.Offset, location.Offset+location.Length)
			reader.Reset(*slice)

			DecodeRow(row, reader)

			readerpObj.cleaner(reader)

			lm.Lock(row.ID, row, R)
			updateMatch := row.Values[filterKey] == filterVal //x
			err := lm.Unlock(row.ID, row, R)
			if err != nil {
				return fmt.Errorf("unlock failed: %w", err)
			}

			fmt.Printf("Row: %+v\n", row)

			if updateMatch {
				if freeSpacePage.PageID == 0 {
					freeSpacePage.PageID = PageID(page.Header.ID)
					freeSpacePage.TempPagePtr = page
					freeSpacePage.FreeMemory = pageObj.ExactFreeMem
				}

				lm.Lock(row.ID, row, W)
				row.Values[updateKey] = updateVal
				err := lm.Unlock(row.ID, row, W)
				if err != nil {
					return fmt.Errorf("unlock failed: %w", err)
				}

				fmt.Printf("Updated Row: %+v", row)
				newRowBytes, err := EncodeRow(row, buffer)
				if err != nil {
					return fmt.Errorf("EncodeRow failed: %w", err)
				}

				if !txOff {
					err = wal.Log(txID, LogTypeUpdate, tableObj.TableName, row.ID, *slice, newRowBytes)
					if err != nil {
						return fmt.Errorf("wal.log failed: %w", err)
					}
				}

				location.Free = true
				freeSpacePage.FreeMemory += location.Length
				nonAddedRows.BytesNeeded += uint16(len(newRowBytes))
				nonAddedRows.Rows = append(nonAddedRows.Rows, newRowBytes)

			}

			slicepObj.cleaner(slice)
			rowpObj.cleaner(row)
			bufferpObj.cleaner(buffer)
		}

		pageObj.Mu.Unlock() // at the end of each page

		if freeSpacePage.PageID != 0 {
			updateInfo.FreeSpaceMapping = freeSpacePage
			updateInfo.NonAddedRow = nonAddedRows

			logger.Log.WithField("updateInfo", updateInfo).Info("Page processed")
			updateInfoChan <- updateInfo
		}

		logger.Log.WithFields(logrus.Fields{"Memlevel": pageObj.Level, "exactFreeMem": pageObj.ExactFreeMem, "offset": pageObj.Offset}).Info("After Modification (PageObj)")
	}

	ReleaseTupleObjs(tupleCtx, row, reader, buffer, slice)
	qe.CtxManager.ReturnContext(tupleCtx)

	logger.Log.Info("processPagesForUpdate (end)")
	return nil
}

func handleLikeInsert(ctx context.Context, accountingCtx *MemoryContext, nonAddedRows chan *NonAddedRows, tableObj *TableObj, tableName string, bpm *BufferPoolManager, tableStats *TableInfo) error {
	logger.Log.Info("handleLikeInsert(update) Started")

	for nonAddedRow := range nonAddedRows {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if nonAddedRow.BytesNeeded >= AVAIL_DATA {
			chunkedRows := ChunkRows(nonAddedRow)

			for _, chunkedRow := range chunkedRows {
				err := findAndUpdate(bpm, tableObj, tableStats, chunkedRow.BytesNeeded, tableName, chunkedRow.Rows)
				if err != nil {
					return fmt.Errorf("findAndUpdate failed: %w", err)
				}
			}

			var nonAddedRowsType = reflect.TypeOf((*NonAddedRows)(nil))
			accountingCtx.Release(nonAddedRowsType, nonAddedRow)
			continue
		}

		err := findAndUpdate(bpm, tableObj, tableStats, nonAddedRow.BytesNeeded, tableName, nonAddedRow.Rows)
		if err != nil {
			return fmt.Errorf("findAndUpdate failed: %w", err)
		}

		var nonAddedRowsType = reflect.TypeOf((*NonAddedRows)(nil))
		accountingCtx.Release(nonAddedRowsType, nonAddedRow)
	}

	logger.Log.Info("handleLikeInsert(update) Completed")
	return nil
}

// Potential Improvement
func ChunkRows(nonAddedRows *NonAddedRows) []*NonAddedRows {
	const maxBytesPerChunk = 2096
	var chunkedRows []*NonAddedRows

	currentChunk := &NonAddedRows{}
	for _, row := range nonAddedRows.Rows {
		rowSize := uint16(len(row))

		if currentChunk.BytesNeeded+rowSize >= maxBytesPerChunk {
			chunkedRows = append(chunkedRows, currentChunk)
			currentChunk = &NonAddedRows{}
		}

		currentChunk.BytesNeeded += rowSize
		currentChunk.Rows = append(currentChunk.Rows, row)
	}

	if len(currentChunk.Rows) > 0 {
		chunkedRows = append(chunkedRows, currentChunk)
	}

	return chunkedRows
}

func getPrimary(tableName string, catalog *Catalog) (string, error) {
	var primary string

	tableInfo, ok := catalog.Tables[tableName]
	if !ok {
		return "", fmt.Errorf("table: %s doesn't exist", tableName)
	}

	for column, columnInfo := range tableInfo.Schema {
		if columnInfo.IsIndex {
			primary = column
			break
		}
	}

	if primary == "" {
		return "", fmt.Errorf("primary doesn't exist")
	}

	return primary, nil
}

func ParsingTableMetadata(stringfied string) (string, map[string]map[string]string) {
	fields := make(map[string]map[string]string)

	tableName := ""
	parts := strings.SplitN(strings.Trim(stringfied, "&"), "&", 2)
	if len(parts) > 1 {
		tableNameParts := strings.SplitN(parts[0], "=", 2)
		if len(tableNameParts) == 2 {
			tableName = tableNameParts[1]
		}
	}

	re := regexp.MustCompile(`&(schema|auth)=\[([^\]]+)\]`)

	matches := re.FindAllStringSubmatch(stringfied, -1)

	for _, match := range matches {
		if len(match) > 2 {
			key := match[1]
			value := match[2]

			fields[key] = CollectKV(value)
		}
	}

	return tableName, fields
}

func ParsingRegistration(stringfied string) map[string]string {
	return CollectKV(stringfied)
}

func CollectKV(stringfied string) map[string]string {
	var currKey []rune
	var currVal []rune
	var collectingKey bool
	var collectingVal bool

	fields := make(map[string]string)

	for _, char := range stringfied {
		if char == '&' { // only collect previous &key=val when a & is seen again.
			if len(currKey) > 0 && len(currVal) > 0 {
				key := string(currKey)
				val := string(currVal)

				fields[key] = val
				currKey = []rune{}
				currVal = []rune{}
			}

			collectingKey = true
			collectingVal = false
			continue
		}

		if char == '=' {
			collectingKey = false
			collectingVal = true
			continue
		}

		if collectingKey {
			currKey = append(currKey, char)
		}

		if collectingVal {
			currVal = append(currVal, char)
		}
	}

	return fields
}

func handleError(err error, msg string) Result {
	return Result{
		Error: err,
		Msg:   msg,
	}
}

func rollbackAndReturn(txId, primary, modifiedColumn, tableName string, walManager *WalManager, engine *QueryEngine, catalog *Catalog, err error, msg string) Result {
	if rollbackErr := walManager.AbortTransaction(txId, primary, modifiedColumn, tableName, engine, catalog); rollbackErr != nil {
		err = fmt.Errorf("AbortTransaction failed: %w", rollbackErr)
	}
	return Result{
		Error: err,
		Msg:   msg,
	}
}

func CreateSchemaString(schemaMap map[string]string) string {
	var schemaStr string

	for k, v := range schemaMap {
		if v == "PRIMARY KEY" {
			schemaStr += fmt.Sprintf("%s(%s),", v, k)
			continue
		}
		schemaStr += k + " " + v + ","
	}

	schemaStr = removeTrailing(schemaStr, ',')
	return schemaStr
}

func removeTrailing(s string, remove rune) string {
	if len(s) > 0 && rune(s[len(s)-1]) == remove {
		return s[:len(s)-1]
	}
	return s
}

func ExecuteQuery(sql string, queryEngine *QueryEngine) (*Result, error) {
	encodedPlan, err := utils.SendSql(sql)
	if err != nil {
		return nil, fmt.Errorf("SendSql Failed: %w", err)
	}

	queryInfo := QueryInfo{Id: GenerateRandomID(), RawPlan: encodedPlan, TransactionOff: false, InduceErr: false}
	resChan := queryEngine.ResultManager.CreatePersonalChan()
	queryEngine.ResultManager.Subscribe(queryInfo.Id, resChan)

	queryEngine.QueryChan <- &queryInfo

	res := <-resChan

	queryEngine.ResultManager.Unsubscribe(queryInfo.Id)
	return res, nil
}

func SendResponse(msg string, conn net.Conn) error {
	writeDeadLine := time.Now().Add(5 * time.Second)
	err := conn.SetWriteDeadline(writeDeadLine)
	if err != nil {
		return fmt.Errorf("SetWriteDeadline failed: %w", err)
	}

	n, err := conn.Write([]byte(msg))
	if err != nil {
		return fmt.Errorf("conn.Write failed: %w", err)
	}

	if n == 0 {
		return errors.New("network write failed, O bytes written")
	}

	return nil
}

func Authenticate(row *RowV2, dbName string) (string, error) {
	secretKey := []byte(os.Getenv("JWT_SECRET"))
	if len(secretKey) == 0 {
		log.Fatal("JWT_SECRET environment variable not set or is empty")
	}

	ttl := time.Hour * 1
	expirationTime := time.Now().Add(ttl).Unix()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userId": fmt.Sprintf("%d", row.ID),
		"dbName": dbName,
		"exp":    expirationTime,
	})

	tokenString, err := token.SignedString(secretKey)
	if err != nil {
		return "", fmt.Errorf("SignedString Failed: %w", err)
	}

	return tokenString, nil
}

func Bookkeeping(email, pass, dbName string, queryEngine *QueryEngine) (*RowV2, error) {
	findSql := fmt.Sprintf("SELECT * FROM `User` WHERE Email = '%s'\n", email)
	encodedPlan, err := utils.SendSql(findSql)
	if err != nil {
		return nil, fmt.Errorf("SendSql failed: %w", err)
	}

	queryInfo := QueryInfo{RawPlan: encodedPlan, TransactionOff: false, InduceErr: false}
	result := queryEngine.QueryProcessingEntry(&queryInfo)
	if result.Error != nil {
		return nil, fmt.Errorf("QueryProcessingEntry failed: %w", result.Error)
	}

	if len(result.Rows) > 0 {
		row := result.Rows[0]
		stored_password := row.Values["Password"]
		stored_dbName := row.Values["DbName"]

		if pass != stored_password || dbName != stored_dbName {
			return nil, errors.New("incorrect credentials")
		}

		return row, nil
	}

	sql := fmt.Sprintf("INSERT INTO `User`(Email, Password, DbName) VALUES ('%s', '%s', '%s')\n", email, pass, dbName)
	encodedPlan, err = utils.SendSql(sql)
	if err != nil {
		return nil, fmt.Errorf("SendSql failed: %w", err)
	}

	queryInfo = QueryInfo{RawPlan: encodedPlan, TransactionOff: false, InduceErr: false}
	result = queryEngine.QueryProcessingEntry(&queryInfo)
	if result.Error != nil {
		return nil, fmt.Errorf("QueryProcessingEntry failed: %w", result.Error)
	}

	return result.Rows[0], nil
}

func undoDelete(log *LogRecord, engine *QueryEngine, catalog *Catalog) error {
	var oldRow RowV2
	buf := bytes.NewReader(log.BeforeImage)

	DecodeRow(&oldRow, buf)

	sql := buildInsertQueryFromMap(log.TableID, oldRow.Values, catalog)

	encodedPlan, err := utils.SendSql(sql)
	if err != nil {
		return fmt.Errorf("SendSqls failed: %w", err)
	}

	queryInfo := QueryInfo{RawPlan: encodedPlan, TransactionOff: false, InduceErr: false}
	result := engine.QueryProcessingEntry(&queryInfo)
	if result.Error != nil {
		return fmt.Errorf("QueryProcessingEntry failed: %w", result.Error)
	}

	return nil
}

func buildInsertQueryFromMap(tableID string, oldRow map[string]string, catalog *Catalog) string {
	var columns []string
	var values []string

	for col, val := range oldRow {
		schema := catalog.Tables[tableID]
		schemaObj := schema.Schema[col]
		columns = append(columns, col)

		if schemaObj.Type == "VARCHAR" {
			values = append(values, fmt.Sprintf("'%v'", val))
			continue
		}
		values = append(values, val)
	}

	query := fmt.Sprintf("INSERT INTO `%s` (%s) VALUES (%s)\n", tableID, strings.Join(columns, ", "), strings.Join(values, ", "))
	return query
}

func undoUpdate(log *LogRecord, engine *QueryEngine, primary, modifiedColumn string) error {
	var oldRow RowV2
	buf := bytes.NewReader(log.BeforeImage)

	DecodeRow(&oldRow, buf)

	oldVal := oldRow.Values[modifiedColumn]
	sql := fmt.Sprintf("UPDATE `%s` SET %s = %s WHERE %s = CAST('%d' AS DECIMAL(20,0))\n", log.TableID, modifiedColumn, oldVal, primary, log.RowID)

	encodedPlan, err := utils.SendSql(sql)
	if err != nil {
		return fmt.Errorf("SendSql failed: %w", err)
	}

	queryInfo := QueryInfo{RawPlan: encodedPlan, TransactionOff: false, InduceErr: false}
	result := engine.QueryProcessingEntry(&queryInfo)
	if result.Error != nil {
		return fmt.Errorf("QueryProcessingEntry failed: %w", result.Error)
	}

	return nil
}

func undoInsert(log *LogRecord, engine *QueryEngine, primary string) error {
	sql := fmt.Sprintf("DELETE FROM `%s` WHERE %s = CAST('%d' AS DECIMAL(20,0))\n", log.TableID, primary, log.RowID)

	encodedPlan, err := utils.SendSql(sql)
	if err != nil {
		return fmt.Errorf("SendSql failed: %w", err)
	}

	queryInfo := QueryInfo{RawPlan: encodedPlan, TransactionOff: false, InduceErr: false}
	result := engine.QueryProcessingEntry(&queryInfo)
	if result.Error != nil {
		return fmt.Errorf("QueryProcessingEntry failed: %w", result.Error)
	}

	return nil
}

func redoInsert(log *LogRecord, engine *QueryEngine) error {

	return nil
}

func redoDelete(log *LogRecord, engine *QueryEngine, catalog *Catalog) error {

	return nil
}

func redoUpdate(log *LogRecord, engine *QueryEngine) error {

	return nil
}

func clearObjectFields(obj any) {
	v := reflect.ValueOf(obj)

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return
	}

	for i := range v.NumField() {
		field := v.Field(i)

		if !field.CanSet() {
			panic("unaddressable field")
		}

		switch field.Kind() {
		case reflect.Slice:
			field.Set(reflect.Zero(field.Type()))
		case reflect.Map:
			field.Set(reflect.Zero(field.Type()))
		case reflect.Ptr:
			field.Set(reflect.Zero(field.Type()))
		case reflect.String:
			field.SetString("")
		case reflect.Bool:
			field.SetBool(false)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			field.SetInt(0)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			field.SetUint(0)
		case reflect.Float32, reflect.Float64:
			field.SetFloat(0)
		case reflect.Struct:
			clearObjectFields(field.Addr().Interface())
		}
	}
}

func SliceBytesExpression(dest *[]byte, source []byte, start, end uint16) {
	for i := start; i < end; i++ {
		*dest = append(*dest, source[i])
	}
}

func DetermineCapacity(alloc AllocationStrategy) int {
	var capacity int

	switch alloc {
	case DefaultAllocation:
		capacity = 1
	case SmallObjectAllocation:
		capacity = 10
	case MediumObjectAllocation:
		capacity = 20
	case LargeObjectAllocation:
		capacity = 100
	}

	return capacity
}
