package controller

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type SometimesWithDebounce struct {
	s     rate.Sometimes
	mu    sync.Mutex
	timer *time.Timer

	lastExec time.Time
}

func NewSometimesWithDebounce(interval time.Duration) *SometimesWithDebounce {
	return &SometimesWithDebounce{
		s: rate.Sometimes{
			First:    0,
			Interval: interval,
		},
	}
}

func (swd *SometimesWithDebounce) Do(callback func()) {
	swd.mu.Lock()
	defer swd.mu.Unlock()

	limited := true
	swd.s.Do(func() {
		limited = false
		callback()
		swd.lastExec = time.Now()
	})

	if limited == true {
		if swd.timer != nil {
			swd.timer.Stop()
		}

		nextExecInterval := swd.s.Interval - time.Now().Sub(swd.lastExec)
		swd.timer = time.AfterFunc(nextExecInterval, func() {
			swd.mu.Lock()
			defer swd.mu.Unlock()
			swd.s.Do(func() {
				callback()
				swd.lastExec = time.Now()
			})
			swd.timer = nil
		})
	}
}
