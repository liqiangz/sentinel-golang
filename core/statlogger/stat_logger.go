package statlogger

import (
	"fmt"
	"github.com/alibaba/sentinel-golang/logging"
	"github.com/alibaba/sentinel-golang/util"
	"strings"
	"sync"
	"sync/atomic"
)

type StatLogger struct {
	loggerName     string
	intervalMillis uint64
	maxEntryCount  int
	data           atomic.Value
	writeChan      chan *StatRollingData
	writer         *StatWriter
	mux            *sync.Mutex
	rollingChan    chan int
}

func (s *StatLogger) Stat(count uint32, args ...string) {
	s.data.Load().(*StatRollingData).CountAndSum(args, count)
}

func (s *StatLogger) writeTaskLoop() {
	for {
		select {
		case srd := <-s.writeChan:
			counter := srd.getCloneDataAndClear()
			if len(counter) == 0 {
				return
			}
			for key, value := range counter {
				b := strings.Builder{}
				_, err := fmt.Fprintf(&b, "%s|%s|%d", util.FormatTimeMillis(srd.timeSlot), key, value)
				if err != nil {
					logging.Warn("[StatLogController] Failed to convert StatData to string", "loggerName", srd.sl.loggerName, "err", err)
					continue
				}
				err = srd.sl.writer.write(b.String())
				if err != nil {
					logging.Warn("[StatLogController] Failed to write StatData", "loggerName", srd.sl.loggerName, "err", err)
					break
				}
			}
			if err := srd.sl.writer.flush(); err != nil {
				logging.Warn("[StatLogController] Failed to flush StatData", "loggerName", srd.sl.loggerName, "err", err)
			}
		}
	}
}

func (s *StatLogger) Rolling() *StatRollingData {
	s.mux.Lock()
	defer s.mux.Unlock()

	prevData := s.data.Load()
	var timeSlot, rollingTimeMillis uint64
	var initCap int
	if prevData == nil {
		now := util.CurrentTimeMillis()
		timeSlot = now - now%s.intervalMillis
		rollingTimeMillis = timeSlot + s.intervalMillis
		initCap = 16
	} else {
		now := util.CurrentTimeMillis()
		timeSlot = now - now%s.intervalMillis
		if timeSlot <= prevData.(*StatRollingData).timeSlot {
			timeSlot = prevData.(*StatRollingData).timeSlot + s.intervalMillis
		}
		rollingTimeMillis = timeSlot + s.intervalMillis
		initCap = len(prevData.(*StatRollingData).counter)
	}

	sr := StatRollingData{
		timeSlot:          timeSlot,
		rollingTimeMillis: rollingTimeMillis,
		counter:           make(map[string]uint32, initCap),
		mux:               new(sync.Mutex),
		sl:                s,
	}
	s.data.Store(&sr)
	if prevData == nil {
		return nil
	}
	return prevData.(*StatRollingData)

}

type StatRollingData struct {
	timeSlot          uint64
	rollingTimeMillis uint64
	counter           map[string]uint32
	mux               *sync.Mutex
	sl                *StatLogger
}

func (s *StatRollingData) CountAndSum(args []string, count uint32) {
	s.mux.Lock()
	defer s.mux.Unlock()
	key := strings.Join(args, "|")
	num, ok := s.counter[key]
	if !ok {
		num = 0
		size := len(s.counter)
		if size < s.sl.maxEntryCount {
			s.counter[key] = num
		} else {
			old := s.counter
			s.counter = make(map[string]uint32, 16)
			clone := StatRollingData{
				timeSlot:          s.timeSlot,
				rollingTimeMillis: s.rollingTimeMillis,
				counter:           old,
				mux:               new(sync.Mutex),
				sl:                s.sl,
			}
			s.sl.writeChan <- &clone
		}
	}
	s.counter[key] = num + count
}

func (s *StatRollingData) getCloneDataAndClear() map[string]uint32 {
	s.mux.Lock()
	defer s.mux.Unlock()
	var counter map[string]uint32
	counter = s.counter
	s.counter = make(map[string]uint32)
	return counter
}
