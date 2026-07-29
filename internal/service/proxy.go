package service

import "github.com/Kcchouette/gowlarr/internal/config"

func effectiveProxyURL(indexerProxyURL string, cfg config.Config) string {
	if indexerProxyURL != "" {
		return indexerProxyURL
	}
	return cfg.HTTPProxy
}
