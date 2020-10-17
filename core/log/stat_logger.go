package log

type StatRollingData struct {
	timeSlot          int
	rollingTimeMillis uint64
}

type StatLogger struct {
	LoggerName      string
	MaxBackupIndex  int
	IntervalMillis  uint64
	MaxEntryCount   int
	counter         map[string]int64
	statRollingData StatRollingData
}

type StatEntry struct {
	keys   []string
	Logger *StatLogger
}

func (s *StatLogger) Stat(args []string) *StatEntry {
	return &StatEntry{args, s}
}

func (s *StatLogger) Rolling() StatRollingData {
	return StatRollingData{}
}
