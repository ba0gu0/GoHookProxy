package metrics

import (
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Metrics struct {
	ActiveConnections  int64
	TotalConnections   int64
	FailedConnections  int64
	ConnectionDuration time.Duration
	BytesSent          int64
	BytesReceived      int64
	AverageLatency     time.Duration
	ErrorDistribution  map[string]int64
	ProtocolStats      map[string]int64
	BandwidthUsage     float64
	P95Latency         time.Duration
	P99Latency         time.Duration
}

type MetricsCollector struct {
	activeConns     int64
	totalConns      int64
	failedConns     int64
	totalDuration   int64
	bytesSent       int64
	bytesReceived   int64
	latencyBuckets  sync.Map
	errorTypes      sync.Map
	protocolStats   sync.Map
	latencySum      int64
	latencyCount    int64
	connectionTimes *sync.Map
	errorCounts     *sync.Map
	bandwidthStats  atomic.Value
	lastUpdateTime  atomic.Value
}

func NewMetricsCollector() *MetricsCollector {
	mc := &MetricsCollector{
		connectionTimes: &sync.Map{},
		errorCounts:     &sync.Map{},
	}
	mc.lastUpdateTime.Store(time.Now())
	return mc
}

func (mc *MetricsCollector) RecordConnection(duration time.Duration) {
	atomic.AddInt64(&mc.totalConns, 1)
	atomic.AddInt64(&mc.totalDuration, int64(duration))
}

func (mc *MetricsCollector) RecordFailure(err error) {
	atomic.AddInt64(&mc.failedConns, 1)
}

func (mc *MetricsCollector) RecordBytes(sent, received int64) {
	atomic.AddInt64(&mc.bytesSent, sent)
	atomic.AddInt64(&mc.bytesReceived, received)
}

func (mc *MetricsCollector) IncrementActiveConnections() {
	atomic.AddInt64(&mc.activeConns, 1)
}

func (mc *MetricsCollector) DecrementActiveConnections() {
	atomic.AddInt64(&mc.activeConns, -1)
}

func (mc *MetricsCollector) GetMetrics() *Metrics {
	return &Metrics{
		ActiveConnections:  atomic.LoadInt64(&mc.activeConns),
		TotalConnections:   atomic.LoadInt64(&mc.totalConns),
		FailedConnections:  atomic.LoadInt64(&mc.failedConns),
		ConnectionDuration: time.Duration(atomic.LoadInt64(&mc.totalDuration)),
		BytesSent:          atomic.LoadInt64(&mc.bytesSent),
		BytesReceived:      atomic.LoadInt64(&mc.bytesReceived),
	}
}

func (mc *MetricsCollector) GetSnapshot() *Metrics {
	if mc.lastUpdateTime.Load() == nil {
		mc.lastUpdateTime.Store(time.Now())
	}

	metrics := &Metrics{
		ActiveConnections:  atomic.LoadInt64(&mc.activeConns),
		TotalConnections:   atomic.LoadInt64(&mc.totalConns),
		FailedConnections:  atomic.LoadInt64(&mc.failedConns),
		ConnectionDuration: time.Duration(atomic.LoadInt64(&mc.totalDuration)),
		BytesSent:          atomic.LoadInt64(&mc.bytesSent),
		BytesReceived:      atomic.LoadInt64(&mc.bytesReceived),
	}

	latencyCount := atomic.LoadInt64(&mc.latencyCount)
	if latencyCount > 0 {
		metrics.AverageLatency = time.Duration(atomic.LoadInt64(&mc.latencySum) / latencyCount)
	}

	metrics.BandwidthUsage = mc.calculateBandwidth()

	mc.lastUpdateTime.Store(time.Now())

	return metrics
}

func (mc *MetricsCollector) RecordLatency(d time.Duration) {
	atomic.AddInt64(&mc.totalDuration, int64(d))
}

func (mc *MetricsCollector) RecordErrorType(err error) {
	errType := reflect.TypeOf(err).String()
	if val, ok := mc.errorTypes.Load(errType); ok {
		mc.errorTypes.Store(errType, val.(int64)+1)
	} else {
		mc.errorTypes.Store(errType, int64(1))
	}
}

func (mc *MetricsCollector) RecordProtocol(proto string) {
	if val, ok := mc.protocolStats.Load(proto); ok {
		mc.protocolStats.Store(proto, val.(int64)+1)
	} else {
		mc.protocolStats.Store(proto, int64(1))
	}
}

func (mc *MetricsCollector) getLatencyPercentile(p float64) time.Duration {
	var buckets []struct {
		latency time.Duration
		count   int64
	}

	mc.latencyBuckets.Range(func(key, value interface{}) bool {
		buckets = append(buckets, struct {
			latency time.Duration
			count   int64
		}{key.(time.Duration), value.(int64)})
		return true
	})

	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].latency < buckets[j].latency
	})

	var total, current int64
	for _, b := range buckets {
		total += b.count
	}

	target := int64(float64(total) * p)
	for _, b := range buckets {
		current += b.count
		if current >= target {
			return b.latency
		}
	}
	return 0
}

func (mc *MetricsCollector) RecordError(err error) {
	if err == nil {
		return
	}
	errType := reflect.TypeOf(err).String()
	if count, ok := mc.errorCounts.LoadOrStore(errType, int64(1)); ok {
		mc.errorCounts.Store(errType, count.(int64)+1)
	}
}

func (mc *MetricsCollector) RecordProtocolUse(protocol string) {
	if count, ok := mc.protocolStats.LoadOrStore(protocol, int64(1)); ok {
		mc.protocolStats.Store(protocol, count.(int64)+1)
	}
}

func (mc *MetricsCollector) calculateBandwidth() float64 {
	lastTime := mc.lastUpdateTime.Load()
	if lastTime == nil {
		return 0
	}

	now := time.Now()
	duration := now.Sub(lastTime.(time.Time))
	if duration == 0 {
		return 0
	}

	totalBytes := atomic.LoadInt64(&mc.bytesSent) + atomic.LoadInt64(&mc.bytesReceived)
	return float64(totalBytes) / duration.Seconds()
}

func (mc *MetricsCollector) GetActiveConnections() int64 {
	return atomic.LoadInt64(&mc.activeConns)
}
