package main

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	sentinel "github.com/alibaba/sentinel-golang/api"
	"github.com/alibaba/sentinel-golang/core/base"
	"github.com/alibaba/sentinel-golang/core/config"
	"github.com/alibaba/sentinel-golang/core/flow"
	"github.com/alibaba/sentinel-golang/logging"
	"github.com/alibaba/sentinel-golang/util"
)

func main() {
	// We should initialize Sentinel first.
	conf := config.NewDefaultConfig()
	// for testing, logging output to console
	conf.Sentinel.Log.Logger = logging.NewConsoleLogger()
	err := sentinel.InitWithConfig(conf)
	if err != nil {
		log.Fatal(err)
	}

	_, err = flow.LoadRules([]*flow.Rule{
		{
			Resource:               "some-test22",
			TokenCalculateStrategy: flow.Direct,
			ControlBehavior:        flow.Reject,
			Threshold:              10,
			StatIntervalInMs:       1000,
		},
		{
			Resource:               "some-test11",
			TokenCalculateStrategy: flow.Direct,
			ControlBehavior:        flow.Reject,
			Threshold:              10,
			StatIntervalInMs:       1000,
		},
	})
	if err != nil {
		log.Fatalf("Unexpected error: %+v", err)
		return
	}

	ch := make(chan struct{})

	for i := 0; i < 10; i++ {
		go func() {
			for {
				e, b := sentinel.Entry("some-test11", sentinel.WithTrafficType(base.Inbound))
				if b != nil {
					fmt.Println(util.CurrentTimeMillis(), "blocked")
					// Blocked. We could get the block reason from the BlockError.
					time.Sleep(time.Duration(rand.Uint64()%10) * time.Millisecond)
				} else {
					// Passed, wrap the logic here.
					fmt.Println(util.CurrentTimeMillis(), "passed")
					time.Sleep(time.Duration(rand.Uint64()%10) * time.Millisecond)

					// Be sure the entry is exited finally.
					e.Exit()
				}

			}
		}()
	}
	for i := 0; i < 10; i++ {
		go func() {
			for {
				e, b := sentinel.Entry("some-test22", sentinel.WithTrafficType(base.Inbound))
				if b != nil {
					fmt.Println(util.CurrentTimeMillis(), "blocked")
					// Blocked. We could get the block reason from the BlockError.
					time.Sleep(time.Duration(rand.Uint64()%10) * time.Millisecond)
				} else {
					// Passed, wrap the logic here.
					fmt.Println(util.CurrentTimeMillis(), "passed")
					time.Sleep(time.Duration(rand.Uint64()%10) * time.Millisecond)

					// Be sure the entry is exited finally.
					e.Exit()
				}

			}
		}()
	}
	<-ch
}
