package upstream

import "sync/atomic"

func (p *Pool) NextTarget() (Target, bool) {
	if len(p.Targets) == 0 {
		return Target{}, false
	}

	index := atomic.AddUint64(&p.next, 1) - 1
	return p.Targets[index%uint64(len(p.Targets))], true
}
