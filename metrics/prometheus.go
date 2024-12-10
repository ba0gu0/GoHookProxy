package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	activeConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "proxy_active_connections",
		Help: "当前活跃连接数",
	})

	totalConnections = promauto.NewCounter(prometheus.CounterOpts{
		Name: "proxy_total_connections",
		Help: "总连接数",
	})

	failedConnections = promauto.NewCounter(prometheus.CounterOpts{
		Name: "proxy_failed_connections",
		Help: "失败连接数",
	})

	connectionDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "proxy_connection_duration_seconds",
		Help:    "连接持续时间分布",
		Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
	})

	bytesSent = promauto.NewCounter(prometheus.CounterOpts{
		Name: "proxy_bytes_sent_total",
		Help: "发送的总字节数",
	})

	bytesReceived = promauto.NewCounter(prometheus.CounterOpts{
		Name: "proxy_bytes_received_total",
		Help: "接收的总字节数",
	})
)

// 更新Prometheus指标
func (mc *MetricsCollector) UpdatePrometheusMetrics() {
	metrics := mc.GetSnapshot()

	activeConnections.Set(float64(metrics.ActiveConnections))
	totalConnections.Add(float64(metrics.TotalConnections))
	failedConnections.Add(float64(metrics.FailedConnections))
	connectionDuration.Observe(metrics.AverageLatency.Seconds())
	bytesSent.Add(float64(metrics.BytesSent))
	bytesReceived.Add(float64(metrics.BytesReceived))
}
