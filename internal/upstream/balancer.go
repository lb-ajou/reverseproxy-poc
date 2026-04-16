package upstream

import (
	"hash/fnv"
	"sync/atomic"
)

func (p *Pool) NextTarget() (Target, bool) {
	if len(p.Targets) == 0 {
		return Target{}, false
	}
	healthyIndexes := p.healthyTargetIndexes()
	if len(healthyIndexes) == 0 {
		return Target{}, false
	}
	index := atomic.AddUint64(&p.next, 1) - 1
	targetIndex := healthyIndexes[index%uint64(len(healthyIndexes))]
	return p.Targets[targetIndex], true
}

func (p *Pool) HashTarget(key string) (Target, bool) {
	healthyIndexes := p.healthyTargetIndexes()
	if len(healthyIndexes) == 0 {
		return Target{}, false
	}
	targetIndex := healthyIndexes[hashIndex(key, len(healthyIndexes))]
	return p.Targets[targetIndex], true
}

func (p *Pool) healthyTargetIndexes() []int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	healthyIndexes := make([]int, 0, len(p.targetState))
	for i := range p.Targets {
		if i >= len(p.targetState) || p.targetState[i].Healthy {
			healthyIndexes = append(healthyIndexes, i)
		}
	}
	return healthyIndexes
}

func hashIndex(key string, size int) int {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(key))
	return int(hasher.Sum64() % uint64(size))
}
