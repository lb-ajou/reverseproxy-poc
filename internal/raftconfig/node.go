package raftconfig

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
)

type NodeOptions struct {
	NodeID        string
	BindAddr      string
	AdvertiseAddr string
	DataDir       string
	Bootstrap     bool
	FSM           *FSM
}

type Node struct {
	Raft        *raft.Raft
	logStore    *raftboltdb.BoltStore
	stableStore *raftboltdb.BoltStore
	transport   *raft.NetworkTransport
}

func (n *Node) Shutdown() error {
	if n == nil {
		return nil
	}

	var errs []error
	if n.Raft != nil {
		if err := n.Raft.Shutdown().Error(); err != nil {
			errs = append(errs, err)
		}
	}
	if n.transport != nil {
		if err := n.transport.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if n.logStore != nil {
		if err := n.logStore.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if n.stableStore != nil {
		if err := n.stableStore.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func NewNode(opts NodeOptions) (*Node, error) {
	if opts.FSM == nil {
		return nil, fmt.Errorf("raft FSM is required")
	}
	if opts.NodeID == "" {
		return nil, fmt.Errorf("raft node ID is required")
	}
	if opts.BindAddr == "" {
		return nil, fmt.Errorf("raft bind address is required")
	}
	if opts.AdvertiseAddr == "" {
		return nil, fmt.Errorf("raft advertise address is required")
	}
	if opts.DataDir == "" {
		return nil, fmt.Errorf("raft data dir is required")
	}
	if err := os.MkdirAll(opts.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create raft data dir: %w", err)
	}

	advertiseAddr, err := net.ResolveTCPAddr("tcp", opts.AdvertiseAddr)
	if err != nil {
		return nil, fmt.Errorf("resolve raft advertise address: %w", err)
	}

	transport, err := raft.NewTCPTransport(opts.BindAddr, advertiseAddr, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("create raft transport: %w", err)
	}
	node := &Node{transport: transport}

	logStore, err := raftboltdb.NewBoltStore(filepath.Join(opts.DataDir, "raft-log.bolt"))
	if err != nil {
		_ = node.Shutdown()
		return nil, fmt.Errorf("create raft log store: %w", err)
	}
	node.logStore = logStore

	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(opts.DataDir, "raft-stable.bolt"))
	if err != nil {
		_ = node.Shutdown()
		return nil, fmt.Errorf("create raft stable store: %w", err)
	}
	node.stableStore = stableStore

	snapshotStore, err := raft.NewFileSnapshotStore(filepath.Join(opts.DataDir, "snapshots"), 2, os.Stderr)
	if err != nil {
		_ = node.Shutdown()
		return nil, fmt.Errorf("create raft snapshot store: %w", err)
	}

	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(opts.NodeID)

	raftNode, err := raft.NewRaft(raftConfig, opts.FSM, logStore, stableStore, snapshotStore, transport)
	if err != nil {
		_ = node.Shutdown()
		return nil, fmt.Errorf("create raft node: %w", err)
	}
	node.Raft = raftNode

	hasState, err := raft.HasExistingState(logStore, stableStore, snapshotStore)
	if err != nil {
		_ = node.Shutdown()
		return nil, fmt.Errorf("inspect raft state: %w", err)
	}
	if opts.Bootstrap && !hasState {
		future := node.Raft.BootstrapCluster(raft.Configuration{
			Servers: []raft.Server{{
				ID:      raft.ServerID(opts.NodeID),
				Address: raft.ServerAddress(opts.AdvertiseAddr),
			}},
		})
		if err := future.Error(); err != nil {
			_ = node.Shutdown()
			return nil, fmt.Errorf("bootstrap raft cluster: %w", err)
		}
	}

	return node, nil
}
