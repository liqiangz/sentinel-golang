package log

import (
	"sync"
	"time"

	"github.com/alibaba/sentinel-golang/util"
)

type StatLogController struct {
	IntervalSeconds int
	MaxEntryCount   int
	statLoggers     map[string]StatLogger
	m               sync.Mutex
}

func (s *StatLogController) initLogger(sl StatLogger) StatLogger {
	s.m.Lock()
	defer s.m.Unlock()
	logger, ok := s.statLoggers[sl.LoggerName]
	if !ok {
		sl.Rolling()
		s.statLoggers[sl.LoggerName] = sl
		logger = sl
	}
	return logger
}

func (s *StatLogController) StatLogRollingTask(sl StatLogger) {
	s.scheduleWriteTask(sl, sl.Rolling())
	s.scheduleNextRollingTask(sl)
}

func (s *StatLogController) scheduleWriteTask(sl StatLogger, srd StatRollingData) {

}

func (s *StatLogController) scheduleNextRollingTask(sl StatLogger) {
	rollingTimeMillis := sl.statRollingData.rollingTimeMillis
	delayMillis := int64(rollingTimeMillis) - int64(util.CurrentTimeMillis())
	if delayMillis > 5 {
		timer := time.NewTimer(time.Duration(delayMillis) * time.Microsecond)
		go func() {
			<-timer.C
			util.CurrentTimeMillis()
			s.StatLogRollingTask(sl)
		}()
	} else if -delayMillis > int64(sl.IntervalMillis) {
		//EagleEye.selfLog("[WARN] unusual delay of statLogger[" + statLogger.getLoggerName() +
		//	"], delay=" + (-delayMillis) + "ms, submit now")
		s.StatLogRollingTask(sl)
	} else {
		s.StatLogRollingTask(sl)

	}
}
