package config

import "errors"

func Validate(cfg AppConfig) error {
	if cfg.ProxyListenAddr == "" {
		return errors.New("proxy listen address is required")
	}
	if cfg.DashboardListenAddr == "" {
		return errors.New("dashboard listen address is required")
	}
	if cfg.ProxyConfigDir == "" {
		return errors.New("proxy config directory is required")
	}
	switch cfg.ConfigStore {
	case "", "file":
		return nil
	case "raft":
		if cfg.RaftNodeID == "" {
			return errors.New("raft node ID is required")
		}
		if cfg.RaftBindAddr == "" {
			return errors.New("raft bind address is required")
		}
		if cfg.RaftAdvertiseAddr == "" {
			return errors.New("raft advertise address is required")
		}
		if cfg.RaftDataDir == "" {
			return errors.New("raft data dir is required")
		}
	default:
		return errors.New("config store must be file or raft")
	}

	return nil
}
