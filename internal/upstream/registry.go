package upstream

import "fmt"

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
