package statlogger

import (
	"github.com/alibaba/sentinel-golang/logging"
	"github.com/go-errors/errors"
	"strconv"
	"sync"
	"time"

	"github.com/alibaba/sentinel-golang/util"
)

var statLoggers = make(map[string]*StatLogger)

var mux = new(sync.Mutex)

const (
	logFlushQueueSize = 60
)

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
		writeChan:      make(chan *StatRollingData, logFlushQueueSize),
		writer:         sw,
	}
	sl.Rolling()
	// Schedule the log flushing task
	go util.RunWithRecover(sl.writeTaskLoop)
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
	sl.writeChan <- sl.Rolling()
	NextRolling(sl)
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
