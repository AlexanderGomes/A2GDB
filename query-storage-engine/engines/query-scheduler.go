package engines

import (
	"sync"
)

type QueryScheduler struct {
	Queries      chan *QueryInfo
	QueryEngine  *QueryEngine
	Notification chan *Result
	ResChan      chan *Result
	Crud         int
	NonCrud      int
	CondCrud     *sync.Cond
	CondNonCrud  *sync.Cond
	Mu           sync.Mutex
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
	return qs
}

func (qs *QueryScheduler) Scheduler() {
	for queryInfo := range qs.Queries {
		go qs.Execute(queryInfo)
	}
}

func (qs *QueryScheduler) Execute(queryInfo *QueryInfo) {
	qs.Mu.Lock()

	if queryInfo.Type == "CRUD" {
		for qs.NonCrud > 0 {
			qs.CondNonCrud.Wait() // unlocks when it shouldn't
		}

		qs.Crud++
		qs.Mu.Unlock()
		go qs.QueryEngine.InlineCruds(queryInfo)
	} else if queryInfo.Type == "NON_CRUD" {
		for qs.Crud > 0 {
			qs.CondCrud.Wait() // unlocks when it shouldn't
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
				qs.CondCrud.Broadcast()
			}
		} else {
			qs.Crud--
			if qs.Crud == 0 {
				qs.CondNonCrud.Broadcast()
			}
		}
		qs.Mu.Unlock()
	}
}
