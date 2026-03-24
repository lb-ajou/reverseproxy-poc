package runtime

import (
	"time"

	"reverseproxy-poc/internal/config"
)

type Snapshot struct {
	AppConfig config.AppConfig
	AppliedAt time.Time
}
