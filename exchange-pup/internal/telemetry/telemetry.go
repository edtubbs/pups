package telemetry

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	ModelLoaded          prometheus.Gauge
	LatestSignalScore    prometheus.Gauge
	PolicyRejects        prometheus.Counter
	RecommendationsTotal *prometheus.CounterVec
	ExecutionsTotal      *prometheus.CounterVec
	PaperPnL             prometheus.Gauge
	APIErrorTotal        *prometheus.CounterVec
	LatencyMS            *prometheus.HistogramVec
}

func New() *Metrics {
	return &Metrics{
		ModelLoaded:          promauto.NewGauge(prometheus.GaugeOpts{Name: "exchange_pup_model_loaded", Help: "1 if model loaded"}),
		LatestSignalScore:    promauto.NewGauge(prometheus.GaugeOpts{Name: "exchange_pup_latest_signal_score", Help: "latest signal confidence"}),
		PolicyRejects:        promauto.NewCounter(prometheus.CounterOpts{Name: "exchange_pup_policy_rejects_total", Help: "policy rejects"}),
		RecommendationsTotal: promauto.NewCounterVec(prometheus.CounterOpts{Name: "exchange_pup_recommendations_total", Help: "recommendations by action"}, []string{"action"}),
		ExecutionsTotal:      promauto.NewCounterVec(prometheus.CounterOpts{Name: "exchange_pup_executions_total", Help: "executions by executor and status"}, []string{"executor", "status"}),
		PaperPnL:             promauto.NewGauge(prometheus.GaugeOpts{Name: "exchange_pup_paper_pnl", Help: "paper pnl"}),
		APIErrorTotal:        promauto.NewCounterVec(prometheus.CounterOpts{Name: "exchange_pup_api_errors_total", Help: "api errors"}, []string{"component"}),
		LatencyMS:            promauto.NewHistogramVec(prometheus.HistogramOpts{Name: "exchange_pup_component_latency_ms", Help: "component latency ms", Buckets: prometheus.DefBuckets}, []string{"component"}),
	}
}

func Handler() http.Handler { return promhttp.Handler() }
