package statlogger

import (
	"bufio"
	"github.com/alibaba/sentinel-golang/core/config"
	"github.com/alibaba/sentinel-golang/logging"
	"github.com/alibaba/sentinel-golang/util"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

type StatWriter struct {
	filePath       string
	maxFileSize    uint64
	maxBackupIndex int
	file           *os.File
	writer         *bufio.Writer
	mux            *sync.Mutex
}

func newStatWriter(fileName string, maxFileSize uint64, maxBackupIndex int) (*StatWriter, error) {
	logDir := config.LogBaseDir()
	if len(logDir) == 0 {
		logDir = config.GetDefaultLogDir()
	}
	if err := util.CreateDirIfNotExists(logDir); err != nil {
		return nil, err
	}
	sw := StatWriter{
		filePath:       filepath.Join(logDir, fileName),
		maxFileSize:    maxFileSize,
		maxBackupIndex: maxBackupIndex,
		mux:            new(sync.Mutex),
	}
	if err := sw.setFile(); err != nil {
		return nil, err
	}
	return &sw, nil
}

func (sw *StatWriter) write(s string) error {
	sw.mux.Lock()
	defer sw.mux.Unlock()
	bs := []byte(s + "\n")
	_, err := sw.writer.Write(bs)
	if err != nil {
		return err
	}
	return nil
}

func (sw *StatWriter) flush() error {
	sw.mux.Lock()
	defer sw.mux.Unlock()
	if err := sw.writer.Flush(); err != nil {
		return err
	}
	if err := sw.rollFileIfSizeExceeded(); err != nil {
		logging.Warn("[StatWriter] Fail to roll file", "err", err)
	}
	return nil
}

func (sw *StatWriter) rollFileIfSizeExceeded() error {
	if sw.file == nil {
		return nil
	}
	stat, err := sw.file.Stat()
	if err != nil {
		return err
	}
	if uint64(stat.Size()) >= sw.maxFileSize {
		if err := sw.rollOver(); err != nil {
			return err
		}
	}
	return nil
}

func (sw *StatWriter) setFile() error {
	mf, err := os.OpenFile(sw.filePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return err
	}
	sw.file = mf
	sw.writer = bufio.NewWriter(mf)
	return nil
}

func (sw *StatWriter) rollOver() error {
	s := sw.filePath + "." + strconv.Itoa(sw.maxBackupIndex)
	fileExists, err := util.FileExists(s)
	if err != nil {
		return err
	}
	if fileExists {
		err = os.Rename(s, s+".deleted")
		if err != nil {
			return err
		}
		err = os.Remove(s + ".deleted")
		if err != nil {
			return err
		}
	}

	for i := sw.maxBackupIndex - 1; i >= 1; i-- {
		fileExists, err := util.FileExists(sw.filePath + "." + strconv.Itoa(i))
		if err != nil {
			return err
		}
		if fileExists {
			err = os.Rename(sw.filePath+"."+strconv.Itoa(i), sw.filePath+"."+strconv.Itoa(i+1))
			if err != nil {
				return err
			}
		}
	}

	fileExists, err = util.FileExists(sw.filePath)
	if err != nil {
		return err
	}
	sw.close()
	if fileExists {
		err = os.Rename(sw.filePath, sw.filePath+"."+strconv.Itoa(1))
		if err != nil {
			return err
		}
	}
	if err = sw.setFile(); err != nil {
		return err
	}
	return nil
}

func (sw *StatWriter) close() {
	err := sw.file.Close()
	if err != nil {
		logging.Warn("[StatWriter] Fail to close file", "err", err)
	}
	sw.file = nil
}
