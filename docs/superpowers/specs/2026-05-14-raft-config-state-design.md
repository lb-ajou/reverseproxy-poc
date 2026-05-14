# Raft Config State Design

## Purpose

This design extends the current reverse proxy POC into a highly available container application by replicating only the desired proxy configuration through HashiCorp Raft.

The design keeps request-time load-balancing state local to each node. Raft is used for strong consistency of configuration changes, not for sharing per-request distribution state.

## Current State Model

The current project uses a file-first state model.

1. `configs/app.json` defines process-local application settings such as proxy listen address, dashboard listen address, and proxy config directory.
2. `configs/proxy/*.json` defines namespace-level proxy configuration.
3. `internal/app.buildSnapshot()` reads the files, validates them, and compiles them into runtime structures.
4. `runtime.Snapshot` stores the active app config, loaded proxy configs, route table, upstream registry, and apply timestamp.
5. `runtime.State` stores one active snapshot and swaps the whole snapshot on reload.
6. `proxy.Handler` reads the current snapshot for each request and selects an upstream target.

The current runtime snapshot contains both durable desired configuration and local runtime state:

- Desired configuration:
  - namespaces from `proxyconfig.LoadedConfig`
  - routes from `proxyconfig.RouteConfig`
  - upstream pools from `proxyconfig.UpstreamPool`
- Compiled projection:
  - `[]route.Route`
  - `*upstream.Registry`
- Local runtime state inside `upstream.Pool`:
  - target health state
  - cached healthy target indexes
  - round-robin cursor
  - least-connection active counters

Only the desired configuration becomes replicated cluster state.

## Design Decision

Use Raft as the source of truth for proxy desired configuration in HA mode.

Keep JSON files as bootstrap, import, export, and development artifacts. Do not treat `configs/proxy/*.json` as the source of truth after a Raft cluster exists.

Keep `configs/app.json` as node-local configuration. It can be extended with cluster settings such as:

- node ID
- Raft bind address
- Raft advertise address
- Raft data directory
- bootstrap flag or bootstrap expectation
- join target
- JSON seed path for brand-new cluster bootstrap

## Non-goals

This design does not replicate request-time load-balancer state, provide cluster-wide health consensus, or make local proxy JSON edits automatically propagate into an existing cluster.

## State Ownership

### Raft-managed state

Raft FSM state owns the canonical desired proxy configuration:

- namespace map
- each namespace's `proxyconfig.Config`
- monotonically increasing config version or applied Raft index
- leader and applied-index metadata exposed by the admin API

This state must be deterministic, serializable, and recoverable from Raft log and snapshots.

### Local derived state

Each node independently derives runtime state from the Raft FSM state:

- loaded config views
- compiled route table
- compiled upstream registry
- active `runtime.Snapshot`

The projection must be deterministic for the same FSM state.

### Local request-time state

Each node keeps the following local and non-replicated:

- health check results
- cached healthy target indexes
- round-robin cursor
- least-connection in-flight counters
- sticky-cookie target choice, which remains client-cookie based
- reverse proxy object cache

Replicating these values through Raft would add write amplification to the hot request path and would not improve correctness for this proxy model.

## Component Model

Introduce a `ConfigStore` boundary between admin APIs and the configuration persistence layer.

Expected responsibilities:

- read namespace desired config
- list namespaces
- submit namespace, route, and upstream pool mutations
- expose the current applied version/index
- notify or trigger runtime projection after committed changes

Implementations:

- `FileConfigStore` for current single-node behavior and compatibility.
- `RaftConfigStore` for HA mode.

The existing admin service must stop calling `os.WriteFile`, `os.Remove`, and `ReloadFromFile()` as its core persistence model in HA mode. HA mode submits commands to `RaftConfigStore`.

## Raft Command Model

Raft commands express user intent, not precompiled runtime objects.

Initial command set:

- `CreateNamespace`
- `DeleteNamespace`
- `CreateRoute`
- `UpdateRoute`
- `DeleteRoute`
- `CreateUpstreamPool`
- `UpdateUpstreamPool`
- `DeleteUpstreamPool`
- `ImportJSONConfig`

Each command includes enough data to validate and apply against the current FSM state. Commands that modify one namespace are applied atomically to that namespace.

## Write Flow

1. Dashboard or Admin API receives a config mutation request.
2. The service validates request shape and obvious schema errors.
3. In HA mode, the service submits a command through `RaftConfigStore`.
4. If the local node is not leader, the store returns a not-leader error with the known leader address.
5. The leader calls `raft.Apply()` with the serialized command.
6. Raft commits the command after quorum replication.
7. The FSM validates the command against the committed current state.
8. The FSM updates the namespace config map if validation passes.
9. The node projects the new FSM state into a `runtime.Snapshot`.
10. `runtime.State.Swap()` publishes the snapshot locally.
11. Health checker lifecycle is updated for the new upstream registry.

Follower nodes receive the same committed log entries and run the same FSM apply path. They also rebuild their local runtime snapshots from the committed desired config.

## Read Flow

Configuration read APIs read from the desired config store.

Examples:

- namespace list
- namespace config
- route definitions
- upstream pool definitions

Runtime read APIs read from the local runtime snapshot.

Examples:

- compiled route table
- compiled upstream registry
- local health status
- local applied timestamp

The API and dashboard label this distinction explicitly. A health status returned by one node is that node's local observation, not a cluster-wide Raft value.

## Bootstrap, Join, and Restore Rules

JSON seed import is allowed only when creating a brand-new Raft cluster with no existing Raft state.

Rules:

1. If the Raft data directory contains existing state, restore from Raft state and do not import JSON seed automatically.
2. If the node is joining an existing cluster, do not import JSON seed.
3. If bootstrapping a new cluster and no Raft state exists, load `configs/proxy/*.json` as the initial FSM state.
4. During normal operation, JSON changes are not auto-reloaded into Raft.
5. A JSON import after bootstrap must be an explicit admin operation that becomes a Raft command.
6. Cluster restore uses Raft log/snapshot state, not local proxy JSON files.

This prevents local files on restarted or newly joined containers from overwriting the cluster's committed desired configuration.

## Error Handling

### No leader or quorum unavailable

Config writes fail when there is no leader or no quorum. The proxy read path continues serving from the last successfully projected local snapshot.

The admin API returns a clear error that distinguishes "request invalid" from "cluster currently cannot commit configuration changes".

### Follower write

Follower writes are not forwarded in the first implementation. A follower returns a not-leader response that includes the known leader address when available.

At the HTTP layer, the admin API returns `409 Conflict` with a stable error code such as `not_raft_leader` and a `leader_address` field. The dashboard client can retry against the leader explicitly.

### Validation failure

Validation must happen before Raft apply for fast feedback and again inside FSM apply for correctness.

The FSM validation pass is required because the state may have changed between request receipt and log commit.

Validation failures must leave FSM state unchanged.

### Projection failure

A committed desired config must always be projectable. If `route.BuildTable()` or `upstream.BuildRegistry()` fails after commit, that indicates a validation gap or implementation bug.

The node must:

- keep serving the previous runtime snapshot
- expose degraded status for the local projection
- log the failed applied index and error
- require a subsequent valid config command to recover

The FSM apply path performs a dry-run projection before accepting a mutation so invalid desired config is not committed.

## JSON File Policy

`configs/app.json` remains active node-local configuration.

`configs/proxy/*.json` is not authoritative in HA mode after cluster bootstrap.

Allowed uses:

- initial seed for a brand-new cluster
- manual import source
- manual export destination
- local development with `FileConfigStore`

Disallowed uses in HA mode:

- automatic reload after cluster state exists
- background file watcher that writes into Raft implicitly
- follower-local file changes affecting cluster state
- restore source for an existing cluster

## Testing Strategy

### FSM tests

- each command applies the expected namespace change
- invalid commands leave state unchanged
- route and upstream validation match current file-backed behavior
- snapshot and restore round trip preserves namespace state
- JSON seed import is accepted only for empty state

### Store and admin service tests

- admin mutations call `ConfigStore` instead of direct file writes in HA mode
- follower write returns leader information
- no-leader writes fail with a cluster availability error
- `FileConfigStore` keeps current single-node behavior

### Runtime projection tests

- the same desired config produces the same route table and upstream registry
- namespace and global ID behavior stays compatible with current `source:localID` model
- route algorithm defaults remain unchanged
- health state starts healthy after a new projection, matching current behavior

### Integration tests

- three-node in-memory Raft cluster replicates config commands
- leader loss elects a new leader and accepts later writes
- new node join ignores local proxy JSON and catches up from Raft
- quorum loss rejects writes while the proxy keeps serving the last local snapshot

## Migration Path

1. Introduce `ConfigStore` without changing runtime behavior.
2. Move current file-backed admin persistence behind `FileConfigStore`.
3. Add deterministic desired-config-to-runtime projection.
4. Add Raft FSM and command types.
5. Add `RaftConfigStore`.
6. Add bootstrap and join rules.
7. Update admin API error responses for leader and quorum cases.
8. Add integration tests for cluster behavior.

This path keeps the existing POC usable while making the persistence model replaceable.

## External References

- HashiCorp Raft package: https://pkg.go.dev/github.com/hashicorp/raft
- HashiCorp Raft apply flow: https://github.com/hashicorp/raft/blob/main/docs/apply.md
