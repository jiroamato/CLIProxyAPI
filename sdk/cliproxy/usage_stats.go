package cliproxy

import (
	"path/filepath"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/usagestats"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/util"
	log "github.com/sirupsen/logrus"
)

// applyUsageStatsConfig opens or closes the persistent usage statistics store
// to match usage-statistics-enabled. The database lives next to the
// credentials, at "<auth-dir>/usage.db". Home mode reports usage to Home
// instead, so nothing is stored locally there.
func (s *Service) applyUsageStatsConfig(cfg *config.Config) {
	enabled := cfg != nil && cfg.UsageStatisticsEnabled && !cfg.Home.Enabled
	path := ""
	if enabled {
		authDir, errResolve := util.ResolveAuthDir(cfg.AuthDir)
		if errResolve != nil {
			log.Errorf("usage statistics disabled: %v", errResolve)
			enabled = false
		} else {
			path = filepath.Join(authDir, usagestats.DefaultFileName)
		}
	}
	if errConfigure := usagestats.Configure(enabled, path); errConfigure != nil {
		log.Errorf("usage statistics disabled: %v", errConfigure)
	}
}
