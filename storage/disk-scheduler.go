package storage

import (
	"errors"
	"fmt"
)

type DiskReq struct {
	Page      Page
	Operation string
}

type DiskResult struct {
	Page     Page
	Response error
}

type DiskScheduler struct {
	RequestChan chan DiskReq
	ResultChan  chan DiskResult
	DiskManager *DiskManagerV2
	Buffer      []*Page
}

func (ds *DiskScheduler) ProccessReq() {
	for req := range ds.RequestChan {
		var result DiskResult
		
		if req.Operation == "WRITE" {
			err := ds.DiskManager.WriteToDisk(req)
			result.Page.ID = req.Page.ID
			if err != nil {
				result.Response = errors.New("unable to write to disk: " + err.Error())
			}
		}
		select {
		case ds.ResultChan <- result:
		default:
			fmt.Println("No listener for result")
		}
	}
}

func (ds *DiskScheduler) AddReq(request DiskReq) {
	ds.RequestChan <- request
}

func NewDiskScheduler(dm *DiskManagerV2) *DiskScheduler {
	diskScheduler := DiskScheduler{
		RequestChan: make(chan DiskReq),
		ResultChan:  make(chan DiskResult),
		DiskManager: dm,
	}

	return &diskScheduler
}
