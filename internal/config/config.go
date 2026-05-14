package config

type AppConfig struct {
	ProxyListenAddr     string `json:"proxyListenAddr"`
	DashboardListenAddr string `json:"dashboardListenAddr"`
	ProxyConfigDir      string `json:"proxyConfigDir"`
	ConfigStore         string `json:"configStore,omitempty"`
	RaftNodeID          string `json:"raftNodeId,omitempty"`
	RaftBindAddr        string `json:"raftBindAddr,omitempty"`
	RaftAdvertiseAddr   string `json:"raftAdvertiseAddr,omitempty"`
	RaftDataDir         string `json:"raftDataDir,omitempty"`
	RaftBootstrap       bool   `json:"raftBootstrap,omitempty"`
	RaftJoinAddr        string `json:"raftJoinAddr,omitempty"`
	RaftJSONSeedDir     string `json:"raftJsonSeedDir,omitempty"`
}

func Default() AppConfig {
	return AppConfig{
		ProxyListenAddr:     ":8080",
		DashboardListenAddr: ":9090",
		ProxyConfigDir:      "configs/proxy",
		ConfigStore:         "file",
	}
}
