package raftconfig

import (
	"encoding/json"

	"reverseproxy-poc/internal/proxyconfig"
)

type CommandType string

const (
	CommandCreateNamespace    CommandType = "create_namespace"
	CommandDeleteNamespace    CommandType = "delete_namespace"
	CommandCreateRoute        CommandType = "create_route"
	CommandUpdateRoute        CommandType = "update_route"
	CommandDeleteRoute        CommandType = "delete_route"
	CommandCreateUpstreamPool CommandType = "create_upstream_pool"
	CommandUpdateUpstreamPool CommandType = "update_upstream_pool"
	CommandDeleteUpstreamPool CommandType = "delete_upstream_pool"
	CommandImportJSONConfig   CommandType = "import_json_config"
)

type Command struct {
	Type      CommandType                   `json:"type"`
	Namespace string                        `json:"namespace,omitempty"`
	RouteID   string                        `json:"route_id,omitempty"`
	PoolID    string                        `json:"pool_id,omitempty"`
	Route     proxyconfig.RouteConfig       `json:"route,omitempty"`
	Pool      proxyconfig.UpstreamPool      `json:"pool,omitempty"`
	Import    map[string]proxyconfig.Config `json:"import,omitempty"`
}

type ApplyResponse struct {
	Error string `json:"error,omitempty"`
}

func EncodeCommand(cmd Command) ([]byte, error) {
	return json.Marshal(cmd)
}

func DecodeCommand(data []byte) (Command, error) {
	var cmd Command
	err := json.Unmarshal(data, &cmd)
	return cmd, err
}
