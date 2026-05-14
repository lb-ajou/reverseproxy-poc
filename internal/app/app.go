package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/raft"

	"reverseproxy-poc/internal/admin"
	"reverseproxy-poc/internal/config"
	"reverseproxy-poc/internal/configstore"
	"reverseproxy-poc/internal/dashboard"
	"reverseproxy-poc/internal/proxy"
	"reverseproxy-poc/internal/proxyconfig"
	"reverseproxy-poc/internal/raftconfig"
	appruntime "reverseproxy-poc/internal/runtime"
	"reverseproxy-poc/internal/upstream"
)

const raftJoinTimeout = 5 * time.Second

type App struct {
	logger           *log.Logger
	configPath       string
	state            *appruntime.State
	mu               sync.Mutex
	runCtx           context.Context
	healthCtx        context.Context
	healthCancel     context.CancelFunc
	healthChecker    *upstream.Checker
	proxyHandler     http.Handler
	dashboardHandler http.Handler
	proxyServer      *http.Server
	dashboardServer  *http.Server
	raftNode         *raftconfig.Node
}

func New(cfg config.AppConfig, configPath string, logger *log.Logger) (*App, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	if err := config.Validate(cfg); err != nil {
		return nil, err
	}

	switch cfg.ConfigStore {
	case "", "file":
		return newFileModeApp(cfg, configPath, logger)
	case "raft":
		return newRaftModeApp(cfg, configPath, logger)
	default:
		return nil, fmt.Errorf("config store must be file or raft")
	}
}

func newFileModeApp(cfg config.AppConfig, configPath string, logger *log.Logger) (*App, error) {
	snapshot, err := buildSnapshot(cfg)
	if err != nil {
		return nil, err
	}

	state := appruntime.NewState(snapshot)
	app := newApp(cfg, configPath, logger, state, snapshot)
	app.dashboardHandler = dashboard.NewHandler(state, admin.New(app))
	app.dashboardServer = newServer(cfg.DashboardListenAddr, app.dashboardHandler)

	return app, nil
}

func newRaftModeApp(cfg config.AppConfig, configPath string, logger *log.Logger) (*App, error) {
	var app *App
	fsm := raftconfig.NewFSMWithConfig(cfg, func(desired configstore.DesiredState) {
		if app == nil {
			return
		}
		snapshot, err := configstore.ProjectSnapshot(cfg, desired)
		if err != nil {
			logger.Printf("failed to project raft configuration: %v", err)
			return
		}
		app.state.Swap(snapshot)
		app.swapHealthChecker(snapshot.Upstreams)
	})

	snapshot, err := configstore.ProjectSnapshot(cfg, fsm.DesiredState())
	if err != nil {
		return nil, err
	}

	state := appruntime.NewState(snapshot)
	app = newApp(cfg, configPath, logger, state, snapshot)

	node, err := raftconfig.NewNode(raftconfig.NodeOptions{
		NodeID:        cfg.RaftNodeID,
		BindAddr:      cfg.RaftBindAddr,
		AdvertiseAddr: cfg.RaftAdvertiseAddr,
		DataDir:       cfg.RaftDataDir,
		Bootstrap:     cfg.RaftBootstrap,
		FSM:           fsm,
	})
	if err != nil {
		return nil, err
	}

	store := raftconfig.NewStore(node.Raft, fsm)
	app.raftNode = node
	if shouldRequestRaftJoin(cfg, node.HasExistingState) {
		ctx, cancel := context.WithTimeout(context.Background(), raftJoinTimeout)
		defer cancel()
		if err := postRaftJoin(ctx, newRaftJoinHTTPClient(), cfg.RaftJoinAddr, cfg.RaftNodeID, cfg.RaftAdvertiseAddr); err != nil {
			_ = node.Shutdown()
			return nil, fmt.Errorf("join raft cluster: %w", err)
		}
	}
	if shouldImportSeed(cfg, node.HasExistingState) {
		seed, err := loadSeedNamespaces(seedConfigDir(cfg))
		if err != nil {
			_ = node.Shutdown()
			return nil, fmt.Errorf("load raft JSON seed: %w", err)
		}
		if len(seed) > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := waitForRaftLeader(ctx, node.Raft); err != nil {
				_ = node.Shutdown()
				return nil, err
			}
			if err := store.ImportJSONConfig(ctx, seed); err != nil {
				_ = node.Shutdown()
				return nil, fmt.Errorf("import raft JSON seed: %w", err)
			}
		}
	}
	app.dashboardHandler = dashboard.NewHandlerWithRaft(state, admin.NewWithStore(store), app)
	app.proxyServer = newServer(cfg.ProxyListenAddr, app.proxyHandler)
	app.dashboardServer = newServer(cfg.DashboardListenAddr, app.dashboardHandler)

	return app, nil
}

func newApp(cfg config.AppConfig, configPath string, logger *log.Logger, state *appruntime.State, snapshot appruntime.Snapshot) *App {
	proxyHandler := proxy.NewHandler(state)
	return &App{
		logger:        logger,
		configPath:    configPath,
		state:         state,
		healthChecker: upstream.NewChecker(snapshot.Upstreams),
		proxyHandler:  proxyHandler,
		proxyServer:   newServer(cfg.ProxyListenAddr, proxyHandler),
	}
}

func (a *App) Snapshot() appruntime.Snapshot {
	return a.state.Snapshot()
}

func (a *App) JoinRaft(ctx context.Context, nodeID, raftAddress string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateRaftServer(nodeID, raftAddress); err != nil {
		return err
	}
	if a.raftNode == nil || a.raftNode.Raft == nil {
		return configstore.NewNotLeaderError("")
	}
	if a.raftNode.Raft.State() != raft.Leader {
		return configstore.NewNotLeaderError(string(a.raftNode.Raft.Leader()))
	}
	err := a.raftNode.Raft.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(raftAddress), 0, 0).Error()
	if errors.Is(err, raft.ErrNotLeader) ||
		errors.Is(err, raft.ErrLeadershipLost) ||
		errors.Is(err, raft.ErrLeadershipTransferInProgress) {
		return configstore.NewNotLeaderError(string(a.raftNode.Raft.Leader()))
	}
	return err
}

func validateRaftServer(nodeID, raftAddress string) error {
	if err := configstore.ValidateIdentifier(nodeID, "node_id"); err != nil {
		return err
	}
	if _, err := net.ResolveTCPAddr("tcp", raftAddress); err != nil {
		return &configstore.StoreError{
			StatusCode: http.StatusBadRequest,
			Code:       "invalid_raft_address",
			Message:    "raft address must be a host:port TCP address",
			Err:        err,
		}
	}
	return nil
}

func buildSnapshot(appCfg config.AppConfig) (appruntime.Snapshot, error) {
	store := configstore.NewFileStore(appCfg)
	desired, err := store.DesiredState(context.Background())
	if err != nil {
		return appruntime.Snapshot{}, fmt.Errorf("load proxy configs: %w", err)
	}

	return configstore.ProjectSnapshot(appCfg, desired)
}

func shouldImportSeed(cfg config.AppConfig, hasExistingState bool) bool {
	return cfg.RaftBootstrap && cfg.RaftJoinAddr == "" && !hasExistingState
}

func seedConfigDir(cfg config.AppConfig) string {
	if cfg.RaftJSONSeedDir != "" {
		return cfg.RaftJSONSeedDir
	}
	return cfg.ProxyConfigDir
}

func loadSeedNamespaces(dir string) (map[string]proxyconfig.Config, error) {
	loaded, err := proxyconfig.LoadDir(dir)
	if err != nil {
		return nil, err
	}
	namespaces := make(map[string]proxyconfig.Config, len(loaded))
	for _, cfg := range loaded {
		if err := configstore.ValidateNamespaceName(cfg.Source); err != nil {
			return nil, err
		}
		namespaces[cfg.Source] = cfg.Config
	}
	return namespaces, nil
}

func shouldRequestRaftJoin(cfg config.AppConfig, hasExistingState bool) bool {
	return cfg.RaftJoinAddr != "" && !hasExistingState
}

type raftJoinRequest struct {
	NodeID      string `json:"node_id"`
	RaftAddress string `json:"raft_address"`
}

type raftJoinErrorResponse struct {
	Message       string `json:"message"`
	Code          string `json:"code,omitempty"`
	LeaderAddress string `json:"leader_address,omitempty"`
}

func postRaftJoin(ctx context.Context, client *http.Client, joinAddr, nodeID, raftAddress string) error {
	if client == nil {
		client = newRaftJoinHTTPClient()
	}
	endpoint, err := raftJoinURL(joinAddr)
	if err != nil {
		return err
	}
	body, err := json.Marshal(raftJoinRequest{NodeID: nodeID, RaftAddress: raftAddress})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	var response raftJoinErrorResponse
	_ = json.NewDecoder(resp.Body).Decode(&response)
	if response.Message == "" {
		response.Message = fmt.Sprintf("raft join request failed with status %d", resp.StatusCode)
	}
	if response.Code == "not_raft_leader" {
		return configstore.NewNotLeaderError(response.LeaderAddress)
	}
	return &configstore.StoreError{
		StatusCode: resp.StatusCode,
		Code:       response.Code,
		Message:    response.Message,
	}
}

func newRaftJoinHTTPClient() *http.Client {
	return &http.Client{Timeout: raftJoinTimeout}
}

func raftJoinURL(joinAddr string) (string, error) {
	parsed, err := url.Parse(joinAddr)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("raft join address must be an absolute dashboard URL")
	}
	if strings.TrimRight(parsed.Path, "/") == "/api/raft/join" {
		return parsed.String(), nil
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/api/raft/join"
	return parsed.String(), nil
}

func waitForRaftLeader(ctx context.Context, node *raft.Raft) error {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if node.State() == raft.Leader {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for raft leader: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (a *App) startHealthChecker(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.healthCancel != nil {
		return
	}

	healthCtx, cancel := context.WithCancel(ctx)
	a.runCtx = ctx
	a.healthCtx = healthCtx
	a.healthCancel = cancel

	if a.healthChecker != nil {
		a.healthChecker.Start(healthCtx)
	}
}

func (a *App) stopHealthChecker() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.healthCancel != nil {
		a.healthCancel()
	}

	a.healthCtx = nil
	a.healthCancel = nil
	a.runCtx = nil
}

func (a *App) swapHealthChecker(registry *upstream.Registry) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.healthCancel != nil {
		a.healthCancel()
		a.healthCancel = nil
	}

	a.healthChecker = upstream.NewChecker(registry)

	if a.runCtx != nil && a.healthChecker != nil {
		healthCtx, cancel := context.WithCancel(a.runCtx)
		a.healthCtx = healthCtx
		a.healthCancel = cancel
		a.healthChecker.Start(healthCtx)
	}
}
