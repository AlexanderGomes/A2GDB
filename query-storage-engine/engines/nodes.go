package engines

import (
	"context"
	"fmt"
	"sync"

	"github.com/scylladb/go-set/strset"
)

const (
	BATCH_THRESHOLD       = 1200
	PROJECTION_WORKERS    = 5
	ROW_COLLECTOR_WORKERS = 10
	FILTER_WORKERS
)

type Node interface {
	GetOutputChan() chan []*RowV2
	initialization(ctx context.Context) error
	GetRes() []*RowV2
	GetNodeType() string
}

type CollectorNode struct {
	Type      string
	InputChan chan []*RowV2
	Rows      *[]*RowV2
}

func (cn CollectorNode) GetNodeType() string {
	return cn.Type
}

func (cn CollectorNode) GetRes() []*RowV2 {
	return *cn.Rows
}

func (cn CollectorNode) GetOutputChan() chan []*RowV2 {
	return nil
}

func (cn CollectorNode) initialization(ctx context.Context) error {
	for rows := range cn.InputChan {
		*cn.Rows = append(*cn.Rows, rows...)
	}

	return nil
}

type TableScanNode struct {
	Type       string
	TableName  string
	Dm         *BufferPoolManager
	OutputChan chan []*RowV2
}

func (tsn TableScanNode) GetNodeType() string {
	return tsn.Type
}

func (tsn TableScanNode) GetRes() []*RowV2 {
	return nil
}

func (tsn TableScanNode) GetOutputChan() chan []*RowV2 {
	return tsn.OutputChan
}

func (tsn TableScanNode) initialization(outerCtx context.Context) error {
	pageChan := make(chan *PageV2, 400)
	errChan := make(chan error, ROW_COLLECTOR_WORKERS+1)

	tableObj, err := GetTableObj(tsn.TableName, tsn.Dm.DiskManager)
	if err != nil {
		return fmt.Errorf("couldn't get table object for table %s, error: %w", tsn.TableName, err)
	}

	tableStats := tsn.Dm.DiskManager.PageCatalog.Tables[tsn.TableName]

	innerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var diskWg sync.WaitGroup
	diskWg.Add(1)
	go func() {
		defer diskWg.Done()
		if err := GetTablePagesFromDisk(outerCtx, innerCtx, pageChan, tableObj, tsn.Dm.PageTable, tableStats.NumOfPages); err != nil {
			errChan <- fmt.Errorf("FullTableScan Failed: %w", err)
			cancel()
		}
	}()

	var rowWg sync.WaitGroup
	for range ROW_COLLECTOR_WORKERS {
		rowWg.Add(1)
		go func() {
			defer rowWg.Done()
			if err := RowCollector(outerCtx, innerCtx, pageChan, tsn.OutputChan, tableObj); err != nil {
				errChan <- fmt.Errorf("RowCollector Failed: %w", err)
				cancel()
			}
		}()
	}

	diskWg.Wait()
	close(pageChan)

	rowWg.Wait()
	close(errChan)
	close(tsn.OutputChan)

	select {
	case err := <-errChan:
		return err
	default:
		return nil
	}
}

type ProjectionNode struct {
	Type       string
	Lm         *LockManager
	Set        *strset.Set
	InputChan  chan []*RowV2
	OutputChan chan []*RowV2
}

func (pn ProjectionNode) GetNodeType() string {
	return pn.Type
}

func (pn ProjectionNode) GetRes() []*RowV2 {
	return nil
}

func (pn ProjectionNode) GetOutputChan() chan []*RowV2 {
	return pn.OutputChan
}

func (pn ProjectionNode) initialization(outerCtx context.Context) error {
	var wg sync.WaitGroup
	errChan := make(chan error, PROJECTION_WORKERS)
	innerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for range PROJECTION_WORKERS {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := Projection(outerCtx, innerCtx, pn.Lm, pn.InputChan, pn.OutputChan, pn.Set); err != nil {
				errChan <- fmt.Errorf("Projection Failed: %w", err)
				cancel()
			}
		}()
	}

	wg.Wait()
	close(pn.OutputChan)
	close(errChan)

	select {
	case err := <-errChan:
		return err
	default:
		return nil
	}
}

type FilterNode struct {
	Type       string
	Lm         *LockManager
	InnerMap   map[string]interface{}
	RefList    map[string]interface{}
	InputChan  chan []*RowV2
	OutputChan chan []*RowV2
}

func (fn FilterNode) GetNodeType() string {
	return fn.Type
}

func (fn FilterNode) GetRes() []*RowV2 {
	return nil
}

func (fn FilterNode) GetOutputChan() chan []*RowV2 {
	return fn.OutputChan
}

func (fn FilterNode) initialization(outerCtx context.Context) error {
	var wg sync.WaitGroup
	errChan := make(chan error, FILTER_WORKERS)

	innerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for range FILTER_WORKERS {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := Filter(outerCtx, innerCtx, fn.Lm, fn.InnerMap, fn.RefList, fn.InputChan, fn.OutputChan); err != nil {
				errChan <- fmt.Errorf("Filter Failed: %w", err)
				cancel()
			}
		}()
	}

	wg.Wait()
	close(fn.OutputChan)
	close(errChan)

	select {
	case err := <-errChan:
		return err
	default:
		return nil
	}
}

type SortNode struct {
	Type       string
	Lm         *LockManager
	InnerMap   map[string]interface{}
	InputChan  chan []*RowV2
	OutputChan chan []*RowV2
}

func (sn SortNode) GetNodeType() string {
	return sn.Type
}

func (sn SortNode) GetRes() []*RowV2 {
	return nil
}

func (sn SortNode) GetOutputChan() chan []*RowV2 {
	return sn.OutputChan
}

func (sn SortNode) initialization(ctx context.Context) error {
	var allRows []*RowV2

	defer close(sn.OutputChan)
	for rows := range sn.InputChan {
		allRows = append(allRows, rows...)
	}

	Sort(ctx, sn.Lm, sn.InnerMap, &allRows, sn.OutputChan)

	return nil
}

type AggregateNode struct {
	Type         string
	Lm           *LockManager
	InnerMap     map[string]interface{}
	GroupKey     string
	SelectedCols []interface{}
	InputChan    chan []*RowV2
	OutputChan   chan []*RowV2
}

func (cn AggregateNode) GetNodeType() string {
	return cn.Type
}

func (cn AggregateNode) GetRes() []*RowV2 {
	return nil
}

func (an AggregateNode) GetOutputChan() chan []*RowV2 {
	return an.OutputChan
}

func (an AggregateNode) initialization(ctx context.Context) error {
	defer close(an.OutputChan)

	var allRows []*RowV2

	for rows := range an.InputChan {
		allRows = append(allRows, rows...)
	}

	err := Aggregate(ctx, an.Lm, an.InnerMap, an.GroupKey, &allRows, an.SelectedCols, an.OutputChan)
	if err != nil {
		return fmt.Errorf("Aggregate failed: %w", err)
	}

	return nil
}
