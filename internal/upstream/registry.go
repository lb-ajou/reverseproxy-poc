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
		if err := registry.addPool(pools[i]); err != nil {
			return nil, err
		}
	}

	return registry, nil
}

func (r *Registry) addPool(pool Pool) error {
	if _, exists := r.pools[pool.GlobalID]; exists {
		return fmt.Errorf("duplicate upstream pool %q", pool.GlobalID)
	}
	r.pools[pool.GlobalID] = copyPool(pool)
	return nil
}

func copyPool(pool Pool) *Pool {
	poolCopy := pool
	ensureActiveCounters(&poolCopy)
	return &poolCopy
}

func ensureActiveCounters(pool *Pool) {
	if len(pool.active) != len(pool.Targets) {
		pool.active = make([]uint64, len(pool.Targets))
	}
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
	ticker, ok := healthTicker(pool)
	if !ok {
		return
	}
	defer ticker.Stop()
	c.runPoolCheck(ctx, pool, ticker)
}

func (c *Checker) runPoolCheck(ctx context.Context, pool *Pool, ticker *time.Ticker) {
	pool.CheckTargets(ctx, c.client)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pool.CheckTargets(ctx, c.client)
		}
	}
}

func healthTicker(pool *Pool) (*time.Ticker, bool) {
	interval, err := pool.HealthInterval()
	if err != nil {
		return nil, false
	}
	return time.NewTicker(interval), true
}
