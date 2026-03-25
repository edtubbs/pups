package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/edtubbs/pups/exchange-pup/internal/config"
	"github.com/edtubbs/pups/exchange-pup/internal/policy"
	"github.com/edtubbs/pups/exchange-pup/internal/storage"
	"github.com/edtubbs/pups/exchange-pup/internal/types"
)

type Runtime interface {
	LatestStatus() map[string]any
	LatestRecommendation() *types.Recommendation
	PredictNow(ctx context.Context) (*types.Recommendation, error)
	ExecuteSignal(ctx context.Context, signalID string) (map[string]any, error)
	ApproveSignal(ctx context.Context, signalID, approver, notes string, approved bool) error
	PaperPerformance(ctx context.Context) (map[string]any, error)
}

type Server struct {
	cfg     config.Config
	store   *storage.Store
	policy  *policy.Engine
	runtime Runtime
	mux     *http.ServeMux
}

func New(cfg config.Config, store *storage.Store, policyEngine *policy.Engine, runtime Runtime) *Server {
	s := &Server{cfg: cfg, store: store, policy: policyEngine, runtime: runtime, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("/health", s.health)
	s.mux.HandleFunc("/status", s.status)
	s.mux.HandleFunc("/config/effective", s.effectiveConfig)
	s.mux.HandleFunc("/signals/latest", s.latestSignal)
	s.mux.HandleFunc("/signals/history", s.signalHistory)
	s.mux.HandleFunc("/positions", s.positions)
	s.mux.HandleFunc("/inventory", s.positions)
	s.mux.HandleFunc("/paper/performance", s.paperPerformance)
	s.mux.HandleFunc("/predict", s.predict)
	s.mux.HandleFunc("/approve/", s.approve)
	s.mux.HandleFunc("/execute/", s.execute)
	s.mux.HandleFunc("/kill-switch/on", s.killOn)
	s.mux.HandleFunc("/kill-switch/off", s.killOff)
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, 200, s.runtime.LatestStatus())
}

func (s *Server) effectiveConfig(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, 200, s.cfg.Redacted())
}

func (s *Server) latestSignal(w http.ResponseWriter, _ *http.Request) {
	r := s.runtime.LatestRecommendation()
	if r == nil {
		s.writeJSON(w, 200, map[string]any{"signal": nil})
		return
	}
	s.writeJSON(w, 200, r)
}

func (s *Server) signalHistory(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	out, err := s.store.LatestSignals(r.Context(), limit)
	if err != nil {
		s.writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	s.writeJSON(w, 200, out)
}

func (s *Server) positions(w http.ResponseWriter, r *http.Request) {
	pp, err := s.runtime.PaperPerformance(r.Context())
	if err != nil {
		s.writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	s.writeJSON(w, 200, pp)
}

func (s *Server) paperPerformance(w http.ResponseWriter, r *http.Request) {
	s.positions(w, r)
}

func (s *Server) predict(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	rec, err := s.runtime.PredictNow(r.Context())
	if err != nil {
		s.writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	s.writeJSON(w, 200, rec)
}

func (s *Server) approve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	signalID := r.URL.Path[len("/approve/"):]
	if signalID == "" {
		s.writeJSON(w, 400, map[string]any{"error": "missing signal_id"})
		return
	}
	approver := r.Header.Get("X-Approver")
	if approver == "" {
		approver = "local"
	}
	if err := s.runtime.ApproveSignal(r.Context(), signalID, approver, "manual approval", true); err != nil {
		s.writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	s.writeJSON(w, 200, map[string]any{"ok": true, "signal_id": signalID})
}

func (s *Server) execute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	signalID := r.URL.Path[len("/execute/"):]
	if signalID == "" {
		s.writeJSON(w, 400, map[string]any{"error": "missing signal_id"})
		return
	}
	res, err := s.runtime.ExecuteSignal(r.Context(), signalID)
	if err != nil {
		s.writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	s.writeJSON(w, 200, res)
}

func (s *Server) killOn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	s.policy.SetKillSwitch(true)
	s.writeJSON(w, 200, map[string]any{"ok": true, "kill_switch": true})
}

func (s *Server) killOff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	s.policy.SetKillSwitch(false)
	s.writeJSON(w, 200, map[string]any{"ok": true, "kill_switch": false})
}
