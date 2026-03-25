package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/artifact"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/attestation"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/blockchain"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/config"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/distribution"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/identity"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/licensing"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/logging"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/policy"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/storage"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/telemetry"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/trust"
)

type Dependencies struct {
	Config       config.Config
	Logger       *logging.Logger
	Storage      *storage.DB
	Metrics      *telemetry.Metrics
	Identity     *identity.Service
	Attestation  *attestation.Service
	Trust        *trust.Service
	Distribution *distribution.Service
	Anchors      *blockchain.Service
	Licensing    *licensing.Service
	Policy       *policy.Engine
}

type Server struct {
	deps Dependencies
	http *http.Server
}

func NewServer(deps Dependencies) (*Server, error) {
	if deps.Logger == nil {
		return nil, errors.New("logger required")
	}
	mux := http.NewServeMux()
	s := &Server{deps: deps, http: &http.Server{Addr: deps.Config.Runtime.HTTPBind, Handler: mux, ReadHeaderTimeout: 5 * time.Second}}

	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /status", s.status)
	mux.HandleFunc("GET /trust/device/", s.getDeviceTrust)
	mux.HandleFunc("GET /trust/provider/", s.getProviderTrust)
	mux.HandleFunc("GET /artifacts/", s.getArtifact)
	mux.HandleFunc("GET /licenses/", s.getLicense)
	mux.HandleFunc("GET /anchors/", s.getAnchor)
	mux.HandleFunc("GET /attestations/", s.getAttestationLatest)

	mux.HandleFunc("POST /device/register", s.postDeviceRegister)
	mux.HandleFunc("POST /device/attest", s.postDeviceAttest)
	mux.HandleFunc("POST /provider/register", s.postProviderRegister)
	mux.HandleFunc("POST /artifact/publish", s.postArtifactPublish)
	mux.HandleFunc("POST /artifact/verify", s.postArtifactVerify)
	mux.HandleFunc("POST /artifact/fetch", s.postArtifactFetch)
	mux.HandleFunc("POST /license/create", s.postLicenseCreate)
	mux.HandleFunc("POST /license/transfer", s.postLicenseTransfer)
	mux.HandleFunc("POST /anchor/create", s.postAnchorCreate)
	mux.HandleFunc("POST /policy/evaluate", s.postPolicyEvaluate)
	mux.HandleFunc("POST /kill-switch/on", s.postKillSwitchOn)
	mux.HandleFunc("POST /kill-switch/off", s.postKillSwitchOff)

	return s, nil
}

func (s *Server) ListenAndServe() error              { return s.http.ListenAndServe() }
func (s *Server) Shutdown(ctx context.Context) error { return s.http.Shutdown(ctx) }

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "digital-asset-signing-pup"})
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"trust_state_summary":     map[string]any{"verification_first": true, "offline_mode": s.deps.Config.Runtime.OfflineVerificationMode},
		"pending_revocations":     0,
		"expiring_entitlements":   0,
		"recent_on_chain_anchors": 0,
		"kill_switch":             s.deps.Config.Policy.KillSwitch,
	})
}

func (s *Server) getDeviceTrust(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/trust/device/")
	rec, err := s.deps.Identity.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	att, _ := s.deps.Attestation.Latest(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]any{"identity": rec, "latest_attestation": att})
}

func (s *Server) getProviderTrust(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/trust/provider/")
	rec, err := s.deps.Identity.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"identity": rec, "trust": "registered"})
}

func (s *Server) getArtifact(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/artifacts/")
	writeJSON(w, http.StatusOK, map[string]any{"artifact_id": id, "status": "lookup pending"})
}

func (s *Server) getLicense(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/licenses/")
	e, err := s.deps.Licensing.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) getAnchor(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/anchors/")
	a, ok := s.deps.Anchors.GetCachedAnchor(id)
	if !ok {
		writeErr(w, http.StatusNotFound, fmt.Errorf("anchor not found"))
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) getAttestationLatest(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/attestations/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "latest" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("expected /attestations/{device_id}/latest"))
		return
	}
	att, err := s.deps.Attestation.Latest(r.Context(), parts[0])
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, att)
}

func (s *Server) postDeviceRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PublicKey string            `json:"public_key"`
		Metadata  map[string]string `json:"metadata"`
		Anchor    bool              `json:"anchor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	rec, err := s.deps.Identity.Register(r.Context(), identity.DeviceIdentity, req.PublicKey, req.Metadata, req.Anchor)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, rec)
}

func (s *Server) postDeviceAttest(w http.ResponseWriter, r *http.Request) {
	var ev attestation.Evidence
	if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	decision, err := s.deps.Attestation.VerifyAndStore(r.Context(), ev)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.deps.Metrics.IncTrustDecision(decision.State != attestation.Trusted)
	writeJSON(w, http.StatusOK, decision)
}

func (s *Server) postProviderRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PublicKey string            `json:"public_key"`
		Metadata  map[string]string `json:"metadata"`
		Anchor    bool              `json:"anchor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	rec, err := s.deps.Identity.Register(r.Context(), identity.ProviderIdentity, req.PublicKey, req.Metadata, req.Anchor)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, rec)
}

func (s *Server) postArtifactPublish(w http.ResponseWriter, r *http.Request) {
	var req struct{ Kind, Digest, Ref, IssuerID string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	a := s.deps.Anchors.EncodeAnchor(req.Kind, req.Digest, req.Ref, req.IssuerID)
	created, err := s.deps.Anchors.CreateAnchor(r.Context(), a)
	if err != nil {
		writeErr(w, http.StatusForbidden, err)
		return
	}
	s.deps.Metrics.IncAnchorsVerified()
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) postArtifactVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Manifest       artifact.Manifest `json:"manifest"`
		ProviderPubKey string            `json:"provider_pub_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	vr := s.deps.Trust.VerifyManifest(r.Context(), req.Manifest, req.ProviderPubKey)
	if !vr.Valid {
		s.deps.Metrics.IncTrustDecision(true)
	}
	writeJSON(w, http.StatusOK, vr)
}

func (s *Server) postArtifactFetch(w http.ResponseWriter, r *http.Request) {
	var req struct{ Digest, Ref string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	res, err := s.deps.Distribution.FetchByDigest(r.Context(), req.Digest, req.Ref)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) postLicenseCreate(w http.ResponseWriter, r *http.Request) {
	var req licensing.Entitlement
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	e, err := s.deps.Licensing.Create(r.Context(), req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) postLicenseTransfer(w http.ResponseWriter, r *http.Request) {
	var req struct{ EntitlementID, ToSubjectID string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.deps.Licensing.Transfer(r.Context(), req.EntitlementID, req.ToSubjectID); err != nil {
		writeErr(w, http.StatusForbidden, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) postAnchorCreate(w http.ResponseWriter, r *http.Request) {
	var rec blockchain.AnchorRecord
	if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	created, err := s.deps.Anchors.CreateAnchor(r.Context(), rec)
	if err != nil {
		writeErr(w, http.StatusForbidden, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) postPolicyEvaluate(w http.ResponseWriter, r *http.Request) {
	var in policy.EvaluationInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if in.Now.IsZero() {
		in.Now = time.Now().UTC()
	}
	out := s.deps.Policy.Evaluate(r.Context(), in)
	if !out.Allow {
		s.deps.Metrics.IncPolicyDeny()
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) postKillSwitchOn(w http.ResponseWriter, _ *http.Request) {
	s.deps.Policy.SetKillSwitch(true)
	writeJSON(w, http.StatusOK, map[string]any{"kill_switch": true})
}
func (s *Server) postKillSwitchOff(w http.ResponseWriter, _ *http.Request) {
	s.deps.Policy.SetKillSwitch(false)
	writeJSON(w, http.StatusOK, map[string]any{"kill_switch": false})
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
