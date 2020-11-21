package hotspot

import (
	"fmt"
	"math"
	"runtime"
	"sync/atomic"

	"github.com/alibaba/sentinel-golang/core/base"
	"github.com/alibaba/sentinel-golang/core/hotspot/cache"
	"github.com/alibaba/sentinel-golang/logging"
	"github.com/alibaba/sentinel-golang/util"
	"github.com/pkg/errors"
)

type TrafficShapingController interface {
	PerformChecking(arg interface{}, batchCount int64) *base.TokenResult

	BoundParamIndex() int

	BoundMetric() *ParamsMetric

	BoundRule() *Rule
}

type baseTrafficShapingController struct {
	r *Rule

	res           string
	metricType    MetricType
	paramIndex    int
	threshold     int64
	specificItems map[interface{}]int64
	durationInSec int64

	metric *ParamsMetric
}

func newBaseTrafficShapingControllerWithMetric(r *Rule, metric *ParamsMetric) *baseTrafficShapingController {
	if r.SpecificItems == nil {
		r.SpecificItems = make(map[interface{}]int64)
	}
	return &baseTrafficShapingController{
		r:             r,
		res:           r.Resource,
		metricType:    r.MetricType,
		paramIndex:    r.ParamIndex,
		threshold:     r.Threshold,
		specificItems: r.SpecificItems,
		durationInSec: r.DurationInSec,
		metric:        metric,
	}
}

func newBaseTrafficShapingController(r *Rule) *baseTrafficShapingController {
	switch r.MetricType {
	case QPS:
		size := 0
		if r.ParamsMaxCapacity > 0 {
			size = int(r.ParamsMaxCapacity)
		} else if r.DurationInSec == 0 {
			size = ParamsMaxCapacity
		} else {
			size = int(math.Min(float64(ParamsMaxCapacity), float64(ParamsCapacityBase*r.DurationInSec)))
		}
		if size <= 0 {
			logging.Warn("[HotSpot newBaseTrafficShapingController] Invalid size of cache, so use default value for ParamsMaxCapacity and ParamsCapacityBase",
				"ParamsMaxCapacity", ParamsMaxCapacity, "ParamsCapacityBase", ParamsCapacityBase)
			size = ParamsMaxCapacity
		}
		metric := &ParamsMetric{
			RuleTimeCounter:  cache.NewTinyLfuCacheMap(size),
			RuleTokenCounter: cache.NewTinyLfuCacheMap(size),
		}
		return newBaseTrafficShapingControllerWithMetric(r, metric)
	case Concurrency:
		size := 0
		if r.ParamsMaxCapacity > 0 {
			size = int(r.ParamsMaxCapacity)
		} else {
			size = ConcurrencyMaxCount
		}
		metric := &ParamsMetric{
			ConcurrencyCounter: cache.NewTinyLfuCacheMap(size),
		}
		return newBaseTrafficShapingControllerWithMetric(r, metric)
	default:
		logging.Error(errors.New("unsupported metric type"), "Ignoring the rule due to unsupported  metric type in Rule.newBaseTrafficShapingController()", "MetricType", r.MetricType.String())
		return nil
	}
}

func (c *baseTrafficShapingController) BoundMetric() *ParamsMetric {
	return c.metric
}

func (c *baseTrafficShapingController) performCheckingForConcurrencyMetric(arg interface{}) *base.TokenResult {
	specificItem := c.specificItems
	initConcurrency := new(int64)
	*initConcurrency = 0
	concurrencyPtr := c.metric.ConcurrencyCounter.AddIfAbsent(arg, initConcurrency)
	if concurrencyPtr == nil {
		// First to access this arg
		return nil
	}
	concurrency := atomic.LoadInt64(concurrencyPtr)
	concurrency++
	if specificConcurrency, existed := specificItem[arg]; existed {
		if concurrency <= specificConcurrency {
			return nil
		}
		msg := fmt.Sprintf("hotspot specific concurrency check blocked, arg: %v", arg)
		return base.NewTokenResultBlockedWithCause(base.BlockTypeHotSpotParamFlow, msg, c.BoundRule(), concurrency)
	}
	threshold := c.threshold
	if concurrency <= threshold {
		return nil
	}
	msg := fmt.Sprintf("hotspot concurrency check blocked, arg: %v", arg)
	return base.NewTokenResultBlockedWithCause(base.BlockTypeHotSpotParamFlow, msg, c.BoundRule(), concurrency)
}

// rejectTrafficShapingController use Reject strategy
type rejectTrafficShapingController struct {
	baseTrafficShapingController
	burstCount int64
}

// rejectTrafficShapingController use Throttling strategy
type throttlingTrafficShapingController struct {
	baseTrafficShapingController
	maxQueueingTimeMs int64
}

func (c *baseTrafficShapingController) BoundRule() *Rule {
	return c.r
}

func (c *baseTrafficShapingController) BoundParamIndex() int {
	return c.paramIndex
}

func (c *rejectTrafficShapingController) PerformChecking(arg interface{}, batchCount int64) *base.TokenResult {
	metric := c.metric
	if metric == nil {
		return nil
	}

	if c.metricType == Concurrency {
		return c.performCheckingForConcurrencyMetric(arg)
	} else if c.metricType > QPS {
		return nil
	}

	timeCounter := metric.RuleTimeCounter
	tokenCounter := metric.RuleTokenCounter
	if timeCounter == nil || tokenCounter == nil {
		return nil
	}

	// calculate available token
	tokenCount := c.threshold
	val, existed := c.specificItems[arg]
	if existed {
		tokenCount = val
	}
	if tokenCount <= 0 {
		msg := fmt.Sprintf("hotspot reject check blocked, threshold is <= 0, arg: %v", arg)
		return base.NewTokenResultBlockedWithCause(base.BlockTypeHotSpotParamFlow, msg, c.BoundRule(), nil)
	}
	maxCount := tokenCount + c.burstCount
	if batchCount > maxCount {
		// return blocked because the batch number is more than max count of rejectTrafficShapingController
		msg := fmt.Sprintf("hotspot reject check blocked, request batch count is more than max token count, arg: %v", arg)
		return base.NewTokenResultBlockedWithCause(base.BlockTypeHotSpotParamFlow, msg, c.BoundRule(), nil)
	}

	for {
		currentTimeInMs := int64(util.CurrentTimeMillis())
		lastAddTokenTimePtr := timeCounter.AddIfAbsent(arg, &currentTimeInMs)
		if lastAddTokenTimePtr == nil {
			// First to fill token, and consume token immediately
			leftCount := maxCount - batchCount
			tokenCounter.AddIfAbsent(arg, &leftCount)
			return nil
		}

		// Calculate the time duration since last token was added.
		passTime := currentTimeInMs - atomic.LoadInt64(lastAddTokenTimePtr)
		if passTime > c.durationInSec*1000 {
			// Refill the tokens because statistic window has passed.
			leftCount := maxCount - batchCount
			oldQpsPtr := tokenCounter.AddIfAbsent(arg, &leftCount)
			if oldQpsPtr == nil {
				// Might not be accurate here.
				atomic.StoreInt64(lastAddTokenTimePtr, currentTimeInMs)
				return nil
			} else {
				// refill token
				restQps := atomic.LoadInt64(oldQpsPtr)
				toAddTokenNum := passTime * tokenCount / (c.durationInSec * 1000)
				newQps := int64(0)
				if toAddTokenNum+restQps > maxCount {
					newQps = maxCount - batchCount
				} else {
					newQps = toAddTokenNum + restQps - batchCount
				}
				if newQps < 0 {
					msg := fmt.Sprintf("hotspot reject check blocked, request batch count is more than available token count, arg: %v", arg)
					return base.NewTokenResultBlockedWithCause(base.BlockTypeHotSpotParamFlow, msg, c.BoundRule(), nil)
				}
				if atomic.CompareAndSwapInt64(oldQpsPtr, restQps, newQps) {
					atomic.StoreInt64(lastAddTokenTimePtr, currentTimeInMs)
					return nil
				}
				runtime.Gosched()
			}
		} else {
			//check whether the rest of token is enough to batch
			oldQpsPtr, found := tokenCounter.Get(arg)
			if found {
				oldRestToken := atomic.LoadInt64(oldQpsPtr)
				if oldRestToken-batchCount >= 0 {
					//update
					if atomic.CompareAndSwapInt64(oldQpsPtr, oldRestToken, oldRestToken-batchCount) {
						return nil
					}
				} else {
					msg := fmt.Sprintf("hotspot reject check blocked, request batch count is more than available token count, arg: %v", arg)
					return base.NewTokenResultBlockedWithCause(base.BlockTypeHotSpotParamFlow, msg, c.BoundRule(), nil)
				}
			}
			runtime.Gosched()
		}
	}
}

func (c *throttlingTrafficShapingController) PerformChecking(arg interface{}, batchCount int64) *base.TokenResult {
	metric := c.metric
	if metric == nil {
		return nil
	}

	if c.metricType == Concurrency {
		return c.performCheckingForConcurrencyMetric(arg)
	} else if c.metricType > QPS {
		return nil
	}

	timeCounter := metric.RuleTimeCounter
	tokenCounter := metric.RuleTokenCounter
	if timeCounter == nil || tokenCounter == nil {
		return nil
	}

	// calculate available token
	tokenCount := c.threshold
	val, existed := c.specificItems[arg]
	if existed {
		tokenCount = val
	}
	if tokenCount <= 0 {
		msg := fmt.Sprintf("hotspot throttling check blocked, threshold is <= 0, arg: %v", arg)
		return base.NewTokenResultBlockedWithCause(base.BlockTypeHotSpotParamFlow, msg, c.BoundRule(), nil)
	}
	intervalCostTime := int64(math.Round(float64(batchCount * c.durationInSec * 1000 / tokenCount)))
	for {
		currentTimeInMs := int64(util.CurrentTimeMillis())
		lastPassTimePtr := timeCounter.AddIfAbsent(arg, &currentTimeInMs)
		if lastPassTimePtr == nil {
			// first access arg
			return nil
		}
		// load the last pass time
		lastPassTime := atomic.LoadInt64(lastPassTimePtr)
		// calculate the expected pass time
		expectedTime := lastPassTime + intervalCostTime

		if expectedTime <= currentTimeInMs || expectedTime-currentTimeInMs < c.maxQueueingTimeMs {
			if atomic.CompareAndSwapInt64(lastPassTimePtr, lastPassTime, currentTimeInMs) {
				awaitTime := expectedTime - currentTimeInMs
				if awaitTime > 0 {
					atomic.StoreInt64(lastPassTimePtr, expectedTime)
					return base.NewTokenResultShouldWait(uint64(awaitTime))
				}
				return nil
			} else {
				runtime.Gosched()
			}
		} else {
			msg := fmt.Sprintf("hotspot throttling check blocked, wait time exceedes max queueing time, arg: %v", arg)
			return base.NewTokenResultBlockedWithCause(base.BlockTypeHotSpotParamFlow, msg, c.BoundRule(), nil)
		}
	}
}
