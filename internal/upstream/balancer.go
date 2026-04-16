package upstream

import "sync/atomic"

func (p *Pool) NextTarget() (Target, bool) {
	if len(p.Targets) == 0 {
		return Target{}, false
	}

	p.mu.RLock()
	healthyIndexes := make([]int, 0, len(p.targetState))
	for i := range p.Targets {
		if i >= len(p.targetState) || p.targetState[i].Healthy {
			healthyIndexes = append(healthyIndexes, i)
		}
	}
	p.mu.RUnlock()

	if len(healthyIndexes) == 0 {
		return Target{}, false
	}

	index := atomic.AddUint64(&p.next, 1) - 1
	targetIndex := healthyIndexes[index%uint64(len(healthyIndexes))]
	return p.Targets[targetIndex], true
}
