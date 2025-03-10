package engines

import (
	"sync"
)

type QueryScheduler struct {
	Queries         chan *QueryInfo
	QueryEngine     *QueryEngine
	FinishedQueries chan *Result
	ResChan         chan *Result
	Crud            int
	NonCrud         int
	Mu              sync.Mutex
}

func (qs *QueryScheduler) Scheduler() {
	for {
		select {
		case queryInfo := <-qs.Queries:
			go qs.Execute(queryInfo)
		case queryFinished := <-qs.FinishedQueries:
			qs.Mu.Lock()
			if queryFinished.QueryTye == "CRUD" {
				qs.Crud--
			} else {
				qs.NonCrud--
			}
			qs.Mu.Unlock()
		}
	}
}

func (qs *QueryScheduler) Execute(queryInfo *QueryInfo) {
	qs.Mu.Lock()

	if queryInfo.Type == "CRUD" && qs.NonCrud == 0 {
		qs.Crud++
		qs.Mu.Unlock()
		go qs.QueryEngine.InlineCruds(queryInfo) // could use channel instead of initializing mulitple goroutines when only one can run at any given time
	} else if queryInfo.Type == "CRUD" && qs.NonCrud > 0 {
		for res := range qs.FinishedQueries {
			if res.QueryTye == "NON_CRUD" {
				qs.NonCrud--
				if qs.NonCrud == 0 {
					qs.Crud++
					qs.Mu.Unlock()
					go qs.QueryEngine.InlineCruds(queryInfo)
					break
				}
			}
		}
	} else if queryInfo.Type == "NON_CRUD" && qs.Crud == 0 {
		qs.NonCrud++
		qs.Mu.Unlock()
		go func() {
			qs.ResChan <- qs.QueryEngine.QueryProcessingEntry(queryInfo)
		}()
	} else if queryInfo.Type == "NON_CRUD" && qs.Crud > 0 {
		for res := range qs.FinishedQueries {
			if res.QueryTye == "CRUD" {
				qs.Crud--
				if qs.Crud == 0 {
					qs.NonCrud++
					qs.Mu.Unlock()
					go func() {
						qs.ResChan <- qs.QueryEngine.QueryProcessingEntry(queryInfo)
					}()
					break
				}
			}
		}
	}
}
