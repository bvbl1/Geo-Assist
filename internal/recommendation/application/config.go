package application

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

func ConfigFromEnv() (Config, error) {
	cfg := DefaultConfig()
	for key, target := range map[string]*time.Duration{"REC_CHECK_INTERVAL": &cfg.CheckInterval, "REC_MODEL_POLL_INTERVAL": &cfg.PollInterval, "REC_BUILD_TIMEOUT": &cfg.BuildTimeout, "REC_SOURCE_TIMEOUT": &cfg.SourceTimeout} {
		if value := os.Getenv(key); value != "" {
			v, err := time.ParseDuration(value)
			if err != nil || v < time.Second || v > 24*time.Hour {
				return cfg, fmt.Errorf("invalid %s", key)
			}
			*target = v
		}
	}
	if value := os.Getenv("REC_AUTO_REBUILD"); value != "" {
		v, err := strconv.ParseBool(value)
		if err != nil {
			return cfg, fmt.Errorf("invalid REC_AUTO_REBUILD")
		}
		cfg.AutoRebuild = v
	}
	return cfg, nil
}
