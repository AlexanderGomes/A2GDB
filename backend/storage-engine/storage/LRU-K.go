package storage

import (
	"container/list"
	"errors"
	"math"
	"sync"
	"time"
)

type LRUKReplacer struct {
	accessHistory *list.List
	frameToElem   map[FrameID]*list.Element
	k             int
	mu            sync.Mutex
}

type AccessEntry struct {
	FrameID     FrameID
	AccessTimes []time.Time
	Frequency   int
}

func (r *LRUKReplacer) RecordAccess(frameID FrameID, avoidInfinitLoop int) {
	accessTime := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()
	if elem, ok := r.frameToElem[frameID]; ok {
		entry := elem.Value.(*AccessEntry)

		entry.AccessTimes = append(entry.AccessTimes, accessTime)
		entry.Frequency++

		r.accessHistory.MoveToFront(elem)
	} else {
		newTime := accessTime.Add(time.Duration(avoidInfinitLoop) * time.Minute)
		accessEntry := &AccessEntry{
			FrameID:     frameID,
			AccessTimes: []time.Time{newTime},
			Frequency:   1 + avoidInfinitLoop,
		}
		elem := r.accessHistory.PushFront(accessEntry)
		r.frameToElem[frameID] = elem
	}

}

func (r *LRUKReplacer) Evict() (FrameID, error) {
	if r.accessHistory.Len() == 0 {
		return -1, nil
	}

	maxDistance := -1
	minFrequency := math.MaxInt64
	var evictedFrameID FrameID
	for frameID := range r.frameToElem {
		distance, err := r.computeBackwardKDistance(frameID)
		if err != nil {
			return -1, err
		}

		elem := r.frameToElem[frameID]
		accessEntry := elem.Value.(*AccessEntry)

		if (accessEntry.Frequency < minFrequency) || (distance > maxDistance) {
			maxDistance = distance
			minFrequency = accessEntry.Frequency
			evictedFrameID = frameID
		}
	}

	r.accessHistory.Remove(r.frameToElem[evictedFrameID])
	delete(r.frameToElem, evictedFrameID)

	return evictedFrameID, nil
}

func (r *LRUKReplacer) computeBackwardKDistance(frameID FrameID) (int, error) {
	elem, ok := r.frameToElem[frameID]
	if !ok {
		return 0, errors.New("frameID not found")
	}

	accessEntry := elem.Value.(*AccessEntry)
	accessTimes := accessEntry.AccessTimes
	if len(accessTimes) < r.k {
		return math.MaxInt32, nil
	}

	var lastAccessTime time.Time
	var kthAccessTime time.Time
	lastAccessTime = accessTimes[len(accessTimes)-1]
	kthAccessTime = accessTimes[len(accessTimes)-r.k]

	return int(lastAccessTime.Sub(kthAccessTime).Seconds()), nil
}

func NewLRUKReplacer(k int) *LRUKReplacer {
	return &LRUKReplacer{
		accessHistory: list.New(),
		frameToElem:   make(map[FrameID]*list.Element),
		k:             k,
		mu:            sync.Mutex{},
	}
}
