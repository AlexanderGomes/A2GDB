package engines

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

const (
	RAM_THRESHOLD = 500 * 1024 * 1024
)

const (
	AUTH = iota + 1
	CREATE_TABLE
	QUERY
)

type QueryEngine struct {
	CtxManager        *ContextManager
	BufferPoolManager *BufferPoolManager
	Lm                *LockManager
	QueryChan         chan *QueryInfo
	ResultManager     *ResultManager
	Scheduler         *QueryScheduler
	InlineMu          sync.Mutex
	SystemStats       *SystemStats
	Config            *QueryEngineConfig
	CanBroadcast      bool
}

type SystemStats struct {
	TotalRAM          uint64
	AvailableRAM      uint64
	RAMUsedPercent    float64
	SwapTotal         uint64
	SwapUsedPercent   float64
	DiskTotal         uint64
	DiskFree          uint64
	DiskUsedPercent   float64
	DiskIOReadBytes   uint64
	DiskIOWriteBytes  uint64
	UnderPressure     bool
	PressureReasoning []string
}

type QueryEngineConfig struct {
	CollectSystemInfoInterval time.Duration
	QueryTimeout              time.Duration // hanging queries
	GarbageCollectionInterval time.Duration
	AllowedRAMConsuption      uint64
	MaxConcurrentQueries      int
}

func (qe *QueryEngine) SystemInfoCollector() {
	ticker := time.NewTicker(qe.Config.CollectSystemInfoInterval)
	defer ticker.Stop()

	var err error
	for range ticker.C {
		qe.SystemStats, err = qe.GetSystemPressureStats()
		if err != nil {
			panic(err)
		}

		// if broadcast is true and underPressure is false
		// it means that the system was previously under pressure
		// but now it has recovered, which gives the permission
		// to notify frozen queries that haven't timed out.
		if !qe.SystemStats.UnderPressure && qe.CanBroadcast {
			qe.Scheduler.CondResourceAvailable.Broadcast()
			qe.CanBroadcast = false
		}
	}
}

type QueryInfo struct {
	Id             uint64
	Type           string
	RawPlan        interface{}
	tableName      string
	TransactionOff bool
	InduceErr      bool
}

func (qe *QueryEngine) QueryManager() {
	var result *Result
	var plan map[string]interface{}

	for queryInfo := range qe.QueryChan {
		queryPlan := queryInfo.RawPlan
		result, plan = unwrapPlannerInfo(queryPlan)

		switch operation := plan["STATEMENT"]; operation {
		case "CREATE_TABLE", "SELECT":
			queryInfo.Type = "NON_CRUD"
			qe.Scheduler.Queries <- queryInfo
		case "INSERT", "DELETE", "UPDATE":
			queryInfo.Type = "CRUD"
			queryInfo.tableName = plan["table"].(string)
			qe.Scheduler.Queries <- queryInfo
		default:
			result.Error = fmt.Errorf("unsupported type: %s", operation)
			result.Msg = "failed"
			qe.ResultManager.GlobalChannel <- result
		}

	}
}

func unwrapPlannerInfo(queryPlan interface{}) (*Result, map[string]interface{}) {
	var result Result

	plan, isMap := queryPlan.(map[string]interface{})
	frontendErr, ok := plan["message"].(string)
	if ok || !isMap {
		result.Error = fmt.Errorf("frontend failed: %s", frontendErr)
		result.Msg = "failed"
	}

	return &result, plan
}

func (qe *QueryEngine) InlineCruds(queryInfo *QueryInfo) {
	qe.InlineMu.Lock()
	defer qe.InlineMu.Unlock()

	tablesMap := qe.BufferPoolManager.Wal.activeTxTable
	tableInfo, ok := tablesMap[queryInfo.tableName]
	if !ok {
		tableInfo = &Table{notification: make(chan bool, 1)}
		tablesMap[queryInfo.tableName] = tableInfo
	}

	if tableInfo.activeTx {
		for <-tableInfo.notification {
			qe.ResultManager.GlobalChannel <- qe.QueryProcessingEntry(queryInfo)
		}
		return
	}

	tableInfo.activeTx = true
	qe.ResultManager.GlobalChannel <- qe.QueryProcessingEntry(queryInfo)
}

func (qe *QueryEngine) QueryProcessingEntry(queryInfo *QueryInfo) *Result {
	var result Result

	plan, isMap := queryInfo.RawPlan.(map[string]interface{})
	frontendErr, ok := plan["message"].(string)
	if ok || !isMap {
		result.Error = fmt.Errorf("frontend failed: %s", frontendErr)
		result.Msg = "failed"
		return &result
	}

	switch operation := plan["STATEMENT"]; operation {
	case "CREATE_TABLE":
		result = qe.handleCreate(plan)
		result.QueryTye = "NON_CRUD"
	case "INSERT":
		result = qe.handleInsert(plan, queryInfo.TransactionOff, queryInfo.InduceErr)
		result.QueryTye = "CRUD"
	case "SELECT":
		result = qe.handleSelect(plan)
		result.QueryTye = "NON_CRUD"
	case "DELETE":
		result = qe.handleDelete(plan, queryInfo.TransactionOff, queryInfo.InduceErr)
		result.QueryTye = "CRUD"
	case "UPDATE":
		result = qe.handleUpdate(plan, queryInfo.TransactionOff, queryInfo.InduceErr)
		result.QueryTye = "CRUD"
	default:
		result.Error = fmt.Errorf("unsupported type: %s", operation)
		result.Msg = "failed"
	}

	result.QueryId = queryInfo.Id

	return &result
}

func (qe *QueryEngine) handleSelect(plan map[string]interface{}) Result {
	var result Result

	nodes, err := ComputeNodes(plan, qe)
	if err != nil {
		return handleError(fmt.Errorf("ComputeNodes Failed: %w", err), "failed")
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(nodes))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, node := range nodes {
		wg.Add(1)
		go func(node Node) {
			defer wg.Done()
			if err := node.initialization(ctx); err != nil {
				errChan <- err
				cancel()
			}
		}(node)
	}

	wg.Wait()
	close(errChan)

	firstError := <-errChan
	if firstError != nil {
		return handleError(fmt.Errorf("handleSelect Failed: %w", firstError), "failed")
	}

	result.Rows = nodes[len(nodes)-1].GetRes()
	result.Msg = "success"

	return result
}

func (qe *QueryEngine) GetSystemPressureStats() (*SystemStats, error) {
	var reasons []string

	underPressure := false

	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}

	if vmStat.Available < qe.Config.AllowedRAMConsuption {
		qe.CanBroadcast = true
		underPressure = true
		reasons = append(reasons, "Low available RAM")
	}

	swapStat, err := mem.SwapMemory()
	if err != nil {
		return nil, err
	}
	if swapStat.UsedPercent > 20 {
		qe.CanBroadcast = true
		underPressure = true
		reasons = append(reasons, "High swap usage")
	}

	diskStat, err := disk.Usage("/")
	if err != nil {
		return nil, err
	}

	if diskStat.UsedPercent > 90 {
		qe.CanBroadcast = true
		underPressure = true
		reasons = append(reasons, "High disk usage")
	}

	initialIO, err := disk.IOCounters()
	if err != nil {
		return nil, err
	}
	time.Sleep(3 * time.Second)
	finalIO, err := disk.IOCounters()
	if err != nil {
		return nil, err
	}

	var readDelta, writeDelta uint64
	for device, initial := range initialIO {
		if final, exists := finalIO[device]; exists {
			readDelta += final.ReadBytes - initial.ReadBytes
			writeDelta += final.WriteBytes - initial.WriteBytes
		}
	}

	if (readDelta+writeDelta)/3 > 20*1024*1024 {
		qe.CanBroadcast = true
		underPressure = true
		reasons = append(reasons, "High disk IO activity")
	}

	fmt.Println("UnderPressure: ", underPressure)
	return &SystemStats{
		TotalRAM:          vmStat.Total,
		AvailableRAM:      vmStat.Available,
		RAMUsedPercent:    vmStat.UsedPercent,
		SwapTotal:         swapStat.Total,
		SwapUsedPercent:   swapStat.UsedPercent,
		DiskTotal:         diskStat.Total,
		DiskFree:          diskStat.Free,
		DiskUsedPercent:   diskStat.UsedPercent,
		DiskIOReadBytes:   readDelta,
		DiskIOWriteBytes:  writeDelta,
		UnderPressure:     underPressure,
		PressureReasoning: reasons,
	}, nil
}
