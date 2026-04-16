package config

type AppConfig struct {
	ProxyListenAddr     string `json:"proxyListenAddr"`
	DashboardListenAddr string `json:"dashboardListenAddr"`
	ProxyConfigDir      string `json:"proxyConfigDir"`
}

func Default() AppConfig {
	return AppConfig{
		ProxyListenAddr:     ":8080",
		DashboardListenAddr: ":9090",
		ProxyConfigDir:      "configs/proxy",
	}
}
