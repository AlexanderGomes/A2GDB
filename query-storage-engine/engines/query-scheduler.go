package engines

import (
	"errors"
	"sync"
	"time"
)

type QueryScheduler struct {
	QueryEngine *QueryEngine

	Queries      chan *QueryInfo
	Notification chan *Result
	ResChan      chan *Result

	Crud                  int
	NonCrud               int
	CondCrud              *sync.Cond
	CondNonCrud           *sync.Cond
	CondResourceAvailable *sync.Cond
	Mu                    sync.Mutex
}

func NewQueryScheduler(notification chan *Result, resChan chan *Result, qe *QueryEngine) *QueryScheduler {
	qs := &QueryScheduler{
		Queries:      make(chan *QueryInfo, 1000),
		Notification: notification,
		ResChan:      resChan,
		QueryEngine:  qe,
	}

	qs.CondCrud = sync.NewCond(&qs.Mu)
	qs.CondNonCrud = sync.NewCond(&qs.Mu)
	qs.CondResourceAvailable = sync.NewCond(&qs.Mu)
	return qs
}

func (qs *QueryScheduler) Scheduler() {
	for queryInfo := range qs.Queries {
		go qs.Execute(queryInfo)
	}
}

func (qs *QueryEngine) WaitForResources() bool {
	timer := time.NewTimer(qs.Config.QueryTimeout)
	defer timer.Stop()

	done := make(chan struct{})

	go func() {
		qs.Scheduler.CondResourceAvailable.Wait()
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

func (qs *QueryScheduler) Execute(queryInfo *QueryInfo) {
	qs.Mu.Lock()

	if qs.QueryEngine.SystemStats.UnderPressure {

		freed := qs.QueryEngine.WaitForResources()
		if !freed {
			res := &Result{
				QueryId:  queryInfo.Id,
				QueryTye: queryInfo.Type,
				Msg:      "resource is scarse in the moment",
				Error:    errors.New("resource is scarse in the moment"),
			}

			qs.ResChan <- res
			return
		}

		//## TODO - Free resources before the query times out.
	}

	if queryInfo.Type == "CRUD" {
		for qs.NonCrud > 0 {
			// unlocks while waiting,
			// which allows for mulitple goroutines
			// of different types to wait together.
			qs.CondNonCrud.Wait()
		}

		qs.Crud++
		qs.Mu.Unlock()

		go qs.QueryEngine.InlineCruds(queryInfo)
	} else if queryInfo.Type == "NON_CRUD" {
		for qs.Crud > 0 {
			qs.CondCrud.Wait()
		}

		qs.NonCrud++
		qs.Mu.Unlock()

		go func() {
			qs.ResChan <- qs.QueryEngine.QueryProcessingEntry(queryInfo)
		}()
	}

}

func (qs *QueryScheduler) Decreaser() {
	for res := range qs.Notification {
		qs.Mu.Lock()
		if res.QueryTye == "NON_CRUD" {
			qs.NonCrud--
			if qs.NonCrud == 0 {
				qs.CondNonCrud.Broadcast()
			}
		} else {
			qs.Crud--
			if qs.Crud == 0 {
				qs.CondCrud.Broadcast()
			}
		}
		qs.Mu.Unlock()
	}
}
