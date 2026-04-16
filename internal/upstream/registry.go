package upstream

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

type Registry struct {
	pools map[string]*Pool
}

func NewRegistry(pools []Pool) (*Registry, error) {
	registry := &Registry{
		pools: make(map[string]*Pool, len(pools)),
	}

	for i := range pools {
		pool := pools[i]
		if _, exists := registry.pools[pool.GlobalID]; exists {
			return nil, fmt.Errorf("duplicate upstream pool %q", pool.GlobalID)
		}

		poolCopy := pool
		registry.pools[pool.GlobalID] = &poolCopy
	}

	return registry, nil
}

func (r *Registry) Get(globalID string) (*Pool, bool) {
	pool, ok := r.pools[globalID]
	return pool, ok
}

func (r *Registry) All() []*Pool {
	pools := make([]*Pool, 0, len(r.pools))
	for _, pool := range r.pools {
		pools = append(pools, pool)
	}

	return pools
}

type Checker struct {
	registry *Registry
	client   *http.Client
}

func NewChecker(registry *Registry) *Checker {
	return &Checker{
		registry: registry,
		client:   http.DefaultClient,
	}
}

func (c *Checker) Start(ctx context.Context) {
	if c == nil || c.registry == nil || c.client == nil {
		return
	}

	for _, pool := range c.registry.All() {
		if pool == nil || pool.HealthCheck == nil {
			continue
		}

		go c.runPool(ctx, pool)
	}
}

func (c *Checker) runPool(ctx context.Context, pool *Pool) {
	pool.CheckTargets(ctx, c.client)

	interval, err := pool.HealthInterval()
	if err != nil {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pool.CheckTargets(ctx, c.client)
		}
	}
}
