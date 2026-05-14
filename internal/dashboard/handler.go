package dashboard

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"reverseproxy-poc/internal/admin"
	"reverseproxy-poc/internal/configstore"
	"reverseproxy-poc/internal/proxyconfig"
	"reverseproxy-poc/internal/runtime"
)

//go:embed static/index.html
var dashboardHTML []byte

type RaftJoiner interface {
	JoinRaft(ctx context.Context, nodeID, raftAddress string) error
}

type raftJoinRequest struct {
	NodeID      string `json:"node_id"`
	RaftAddress string `json:"raft_address"`
}

func NewHandler(state *runtime.State, service admin.Service) http.Handler {
	return NewHandlerWithRaft(state, service, nil)
}

func NewHandlerWithRaft(state *runtime.State, service admin.Service, joiner RaftJoiner) http.Handler {
	if service == nil {
		panic("dashboard admin service is required")
	}

	mux := http.NewServeMux()
	if joiner != nil {
		registerRaftAPI(mux, joiner)
	}
	registerConfigAPI(mux, service)
	registerRuntimeAPI(mux, state)
	registerSPA(mux)

	return mux
}

func registerRaftAPI(mux *http.ServeMux, joiner RaftJoiner) {
	mux.HandleFunc("/api/raft/join", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeAPIError(w, newMethodNotAllowedError())
			return
		}

		var request raftJoinRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, err)
			return
		}
		if request.NodeID == "" || request.RaftAddress == "" {
			writeAPIError(w, &admin.APIError{
				StatusCode: http.StatusBadRequest,
				Code:       "invalid_request",
				Message:    "node_id and raft_address are required",
			})
			return
		}

		if err := joiner.JoinRaft(r.Context(), request.NodeID, request.RaftAddress); err != nil {
			writeRaftJoinError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func writeRaftJoinError(w http.ResponseWriter, err error) {
	var storeErr *configstore.StoreError
	if errors.As(err, &storeErr) {
		writeAPIError(w, &admin.APIError{
			StatusCode:    storeErr.StatusCode,
			Code:          storeErr.Code,
			Message:       storeErr.Message,
			LeaderAddress: storeErr.LeaderAddress,
			Err:           storeErr.Err,
		})
		return
	}
	writeAPIError(w, err)
}

func registerSPA(mux *http.ServeMux) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(dashboardHTML))
	})
}

func newMethodNotAllowedError() *admin.APIError {
	return &admin.APIError{
		StatusCode: http.StatusMethodNotAllowed,
		Message:    "method not allowed",
	}
}

type errorResponse struct {
	Message       string                        `json:"message"`
	Code          string                        `json:"code,omitempty"`
	LeaderAddress string                        `json:"leader_address,omitempty"`
	Errors        []proxyconfig.ValidationError `json:"errors,omitempty"`
}

func writeJSON(w http.ResponseWriter, value interface{}) {
	writeJSONStatus(w, http.StatusOK, value)
}

func writeJSONStatus(w http.ResponseWriter, statusCode int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAPIError(w http.ResponseWriter, err error) {
	var adminErr *admin.APIError
	if errors.As(err, &adminErr) {
		response := errorResponse{
			Message:       adminErr.Message,
			Code:          adminErr.Code,
			LeaderAddress: adminErr.LeaderAddress,
			Errors:        adminErr.ValidationErrors,
		}

		var storeErr *configstore.StoreError
		if errors.As(adminErr.Err, &storeErr) {
			if response.Code == "" {
				response.Code = storeErr.Code
			}
			if response.LeaderAddress == "" {
				response.LeaderAddress = storeErr.LeaderAddress
			}
		}

		writeJSONStatus(w, adminErr.StatusCode, response)
		return
	}

	writeJSONStatus(w, http.StatusInternalServerError, errorResponse{Message: "internal server error"})
}
