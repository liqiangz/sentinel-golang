package statlogger

import (
	"fmt"
	"github.com/alibaba/sentinel-golang/logging"
	"github.com/go-errors/errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alibaba/sentinel-golang/util"
)

var statLoggers = make(map[string]*StatLogger)

var mux = new(sync.Mutex)

// NewStatLogger constructs a NewStatLogger
func NewStatLogger(loggerName string, maxBackupIndex int, intervalMillis uint64, maxEntryCount int, maxFileSize uint64) *StatLogger {
	sw, err := newStatWriter(loggerName, maxFileSize, maxBackupIndex)
	if err != nil {
		return nil
	}
	sl := &StatLogger{
		loggerName:     loggerName,
		intervalMillis: intervalMillis,
		maxEntryCount:  maxEntryCount,
		mux:            new(sync.Mutex),
		writer:         sw,
	}
	sl.Rolling()
	AddLogger(sl)
	return sl
}

func AddLogger(sl *StatLogger) *StatLogger {
	util.CurrentTimeMillis()
	mux.Lock()
	defer mux.Unlock()
	logger, ok := statLoggers[sl.loggerName]
	if !ok {
		logger = sl
		statLoggers[sl.loggerName] = logger
		go StatLogRolling(logger)
	}
	return logger
}

func StatLogRolling(sl *StatLogger) {
	defer func() {
		if err := recover(); err != nil {
			logging.Error(errors.Errorf("%+v", err), "unexpected panic")
		}
	}()
	go WriteStatLog(sl.Rolling())
	NextRolling(sl)
}

func WriteStatLog(srd *StatRollingData) {
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
			return
		}
	}
	if err := srd.sl.writer.flush(); err != nil {
		logging.Warn("[StatLogController] Failed to flush StatData", "loggerName", srd.sl.loggerName, "err", err)
	}
}

func NextRolling(sl *StatLogger) {
	rollingTimeMillis := sl.data.Load().(*StatRollingData).rollingTimeMillis
	delayMillis := int64(rollingTimeMillis) - int64(util.CurrentTimeMillis())
	if delayMillis > 5 {
		timer := time.NewTimer(time.Duration(delayMillis) * time.Millisecond)
		<-timer.C
		StatLogRolling(sl)
	} else if -delayMillis > int64(sl.intervalMillis) {
		logging.Warn("[StatLogController] unusual delay of statLogger[" + sl.loggerName + "], " +
			"delay=" + strconv.FormatInt(-delayMillis, 10) + "ms, submit now")
		StatLogRolling(sl)
	} else {
		StatLogRolling(sl)
	}
}
