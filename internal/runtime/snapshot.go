package runtime

import (
	"time"

	"reverseproxy-poc/internal/config"
)

type Snapshot struct {
	Config    config.Config
	AppliedAt time.Time
}
