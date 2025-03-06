package engines

import "sync"

type ResultManager struct {
	mu                sync.Mutex
	SubscribedQueries map[uint64]chan *Result // buffer the channel to 1 // avoid blocking
	GlobalChannel     chan *Result
}

func (rm *ResultManager) ResultCollector() {
	for res := range rm.GlobalChannel {
		subQueryChan, ok := rm.SubscribedQueries[res.QueryId]
		if !ok {
			panic("queryId not subscribed, can't deliver message")
		}

		subQueryChan <- res
	}
}

func (rm *ResultManager) Subscribe(queryId uint64, resChan chan *Result) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.SubscribedQueries[queryId] = resChan
}

func (rm *ResultManager) Unsubscribe(queryId uint64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	delete(rm.SubscribedQueries, queryId)
}

func (rm *ResultManager) CreatePersonalChan() chan *Result {
	return make(chan *Result, 1)
}
