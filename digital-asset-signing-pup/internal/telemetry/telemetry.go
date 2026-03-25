package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type Metrics struct {
	trustDecisions int64
	policyDenies   int64
	anchors        int64
	mu             sync.Mutex
	http           *http.Server
}

func New() *Metrics { return &Metrics{} }

func (m *Metrics) IncTrustDecision(deny bool) {
	atomic.AddInt64(&m.trustDecisions, 1)
	if deny {
		atomic.AddInt64(&m.policyDenies, 1)
	}
}
func (m *Metrics) IncPolicyDeny()      { atomic.AddInt64(&m.policyDenies, 1) }
func (m *Metrics) IncAnchorsVerified() { atomic.AddInt64(&m.anchors, 1) }

func (m *Metrics) ListenAndServe(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "trust_decisions_total %d\n", atomic.LoadInt64(&m.trustDecisions))
		fmt.Fprintf(w, "policy_denies_total %d\n", atomic.LoadInt64(&m.policyDenies))
		fmt.Fprintf(w, "anchors_verified_total %d\n", atomic.LoadInt64(&m.anchors))
	})
	m.mu.Lock()
	m.http = &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	m.mu.Unlock()
	return m.http.ListenAndServe()
}

func (m *Metrics) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.http == nil {
		return nil
	}
	return m.http.Shutdown(ctx)
}
