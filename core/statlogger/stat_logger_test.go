package statlogger

import (
	"github.com/alibaba/sentinel-golang/core/log"
	"strconv"
	"testing"
	"time"
)

func Test_matchArg(t *testing.T) {
	t.Run("Test_matchArg", func(t *testing.T) {
		for i := 0; i < 10000; i++ {
			time.Sleep(1 * time.Second)
			for i := 0; i < 100; i++ {
				log.BlockLogger.Stat(uint32(i), "13", "23", strconv.Itoa(i))
			}
		}

	})

}
