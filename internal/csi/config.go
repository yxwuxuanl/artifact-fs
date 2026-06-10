package csi

import (
	"path/filepath"
	"time"

	"github.com/cloudflare/artifact-fs/internal/daemon"
	"github.com/cloudflare/artifact-fs/internal/model"
)

func fillPaths(root string, cfg *model.RepoConfig) {
	if cfg.GitDir == "" {
		cfg.GitDir = filepath.Join(root, "repos", string(cfg.ID), "git")
	}
	if cfg.OverlayDir == "" {
		cfg.OverlayDir = filepath.Join(root, "overlays", string(cfg.ID))
	}
	if cfg.BlobCacheDir == "" {
		cfg.BlobCacheDir = filepath.Join(root, "cache", "blobs", string(cfg.ID))
	}
	if cfg.MetaDBPath == "" {
		cfg.MetaDBPath = filepath.Join(root, "meta", string(cfg.ID)+".sqlite")
	}
	if cfg.OverlayDBPath == "" {
		cfg.OverlayDBPath = filepath.Join(cfg.OverlayDir, "meta.sqlite")
	}
}

func paramsToRepoConfig(root, name string, params map[string]string) model.RepoConfig {
	remoteURL := params["remoteURL"]
	branch := params["branch"]
	if branch == "" {
		branch = "main"
	}
	refreshInterval := params["refreshInterval"]
	if refreshInterval == "" {
		refreshInterval = "30s"
	}
	refresh, err := daemon.ParseRefresh(refreshInterval)
	if err != nil {
		refresh = 30 * time.Second
	}

	cfg := model.RepoConfig{
		ID:              model.RepoID(name),
		Name:            name,
		RemoteURL:       remoteURL,
		Branch:          branch,
		RefreshInterval: refresh,
		Enabled:         true,
	}
	fillPaths(root, &cfg)
	return cfg
}

func volumeContextToRepoConfig(root, volumeID string, ctx map[string]string) model.RepoConfig {
	return paramsToRepoConfig(root, volumeID, ctx)
}

func paramsToVolumeContext(cfg model.RepoConfig) map[string]string {
	return map[string]string{
		"remoteURL":       cfg.RemoteURL,
		"branch":          cfg.Branch,
		"refreshInterval": cfg.RefreshInterval.String(),
	}
}
