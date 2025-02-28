package engines

import (
	"context"
	"fmt"
	"sync"

	"github.com/scylladb/go-set/strset"
)

const (
	BATCH_THRESHOLD       = 20
	PROJECTION_WORKERS    = 5
	ROW_COLLECTOR_WORKERS = 10
	FILTER_WORKERS
)

// have a final node that puts everything in order

type Node interface {
	GetOutputChan() chan []*RowV2
	initialization(ctx context.Context) error
}

type CollectorNode struct {
	InputChan chan []*RowV2
}

func (cn CollectorNode) GetOutputChan() chan []*RowV2 {
	return nil
}

func (cn CollectorNode) initialization(ctx context.Context) error {
	var allRows []*RowV2
	for rows := range cn.InputChan {
		allRows = append(allRows, rows...)
	}

	fmt.Println(len(allRows))
	return nil
}

type TableScanNode struct {
	TableName  string
	Dm         *BufferPoolManager
	OutputChan chan []*RowV2
}

func (tsn TableScanNode) GetOutputChan() chan []*RowV2 {
	return tsn.OutputChan
}

func (tsn TableScanNode) initialization(ctx context.Context) error {
	pageChan := make(chan *PageV2, 100)
	errChan := make(chan error, 2)

	tableObj, err := GetTableObj(tsn.TableName, tsn.Dm.DiskManager)
	if err != nil {
		return fmt.Errorf("couldn't get table object for table %s, error: %w", tsn.TableName, err)
	}

	tableStats := tsn.Dm.DiskManager.PageCatalog.Tables[tsn.TableName]

	var diskWg sync.WaitGroup
	diskWg.Add(1)
	go func() {
		defer diskWg.Done()
		if err := GetTablePagesFromDisk(ctx, pageChan, tableObj, tsn.Dm.PageTable, tableStats.NumOfPages); err != nil {
			errChan <- fmt.Errorf("FullTableScan Failed: %w", err)
		}
	}()

	go func() {
		diskWg.Wait()
		close(pageChan)
	}()

	var rowWg sync.WaitGroup
	for range ROW_COLLECTOR_WORKERS {
		rowWg.Add(1)
		go func() {
			defer rowWg.Done()
			if err := RowCollector(ctx, pageChan, tsn.OutputChan, tableObj); err != nil {
				errChan <- fmt.Errorf("RowCollector Failed: %w", err)
			}
		}()
	}

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
	Set        *strset.Set
	InputChan  chan []*RowV2
	OutputChan chan []*RowV2
}

func (pn ProjectionNode) GetOutputChan() chan []*RowV2 {
	return pn.OutputChan
}

func (pn ProjectionNode) initialization(ctx context.Context) error {
	var wg sync.WaitGroup

	for range PROJECTION_WORKERS {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Projection(ctx, pn.InputChan, pn.OutputChan, pn.Set)
		}()
	}

	wg.Wait()
	close(pn.OutputChan)

	return nil
}

type FilterNode struct {
	InnerMap   map[string]interface{}
	RefList    map[string]interface{}
	InputChan  chan []*RowV2
	OutputChan chan []*RowV2
}

func (fn FilterNode) GetOutputChan() chan []*RowV2 {
	return fn.OutputChan
}

func (fn FilterNode) initialization(ctx context.Context) error {
	var wg sync.WaitGroup
	errChan := make(chan error, FILTER_WORKERS)

	for range FILTER_WORKERS {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := Filter(ctx, fn.InnerMap, fn.RefList, fn.InputChan, fn.OutputChan); err != nil {
				errChan <- fmt.Errorf("RowCollector Failed: %w", err)
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
	InnerMap   map[string]interface{}
	InputChan  chan []*RowV2
	OutputChan chan []*RowV2
}

func (sn SortNode) GetOutputChan() chan []*RowV2 {
	return sn.OutputChan
}

func (sn SortNode) initialization(ctx context.Context) error {
	var allRows []*RowV2

	for rows := range sn.InputChan {
		allRows = append(allRows, rows...)
	}

	Sort(ctx, sn.InnerMap, &allRows, sn.OutputChan)
	close(sn.OutputChan)

	return nil
}

type AggregateNode struct {
	InnerMap     map[string]interface{}
	GroupKey     string
	SelectedCols []interface{}
	InputChan    chan []*RowV2
	OutputChan   chan []*RowV2
}

func (an AggregateNode) GetOutputChan() chan []*RowV2 {
	return an.OutputChan
}

func (an AggregateNode) initialization(ctx context.Context) error {
	var allRows []*RowV2

	for rows := range an.InputChan {
		allRows = append(allRows, rows...)
	}

	err := Aggregate(ctx, an.InnerMap, an.GroupKey, &allRows, an.SelectedCols, an.OutputChan)
	if err != nil {
		return fmt.Errorf("Aggregate failed: %w", err)
	}

	close(an.OutputChan)

	return nil
}
