package upstream

type Pool struct {
	GlobalID    string
	LocalID     string
	Source      string
	Targets     []Target
	HealthCheck *HealthCheck
	next        uint64
}

type Target struct {
	Raw string
}

type HealthCheck struct {
	Path         string
	Interval     string
	Timeout      string
	ExpectStatus int
}
