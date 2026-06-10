package csi

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/cloudflare/artifact-fs/internal/auth"
	"github.com/cloudflare/artifact-fs/internal/fusefs"
	"github.com/cloudflare/artifact-fs/internal/gitstore"
	"github.com/cloudflare/artifact-fs/internal/hydrator"
	"github.com/cloudflare/artifact-fs/internal/model"
	"github.com/cloudflare/artifact-fs/internal/overlay"
	"github.com/cloudflare/artifact-fs/internal/registry"
	"github.com/cloudflare/artifact-fs/internal/snapshot"
)

type mountManager struct {
	root     string
	logger   *slog.Logger
	mu       sync.Mutex
	git      *gitstore.Store
	registry *registry.Store
	repos    map[model.RepoID]*csiRepo
}

type csiRepo struct {
	cfg      model.RepoConfig
	snapshot *snapshot.Store
	overlay  *overlay.Store
	hydrator *hydrator.Service
	resolver *fusefs.Resolver
	engine   *fusefs.Engine
	refcnt   int
	mounts   map[string]fusefs.MountedFS // targetPath -> mfs
}

func newMountManager(root string, gs *gitstore.Store, reg *registry.Store, logger *slog.Logger) *mountManager {
	return &mountManager{
		root:     root,
		logger:   logger,
		git:      gs,
		registry: reg,
		repos:    map[model.RepoID]*csiRepo{},
	}
}

func (m *mountManager) PrepareRepo(ctx context.Context, cfg model.RepoConfig) error {
	if err := model.ValidateRepoName(cfg.Name); err != nil {
		return err
	}
	cfg.RemoteURLRedacted = auth.RedactRemoteURL(cfg.RemoteURL)
	if cfg.RefreshInterval <= 0 {
		cfg.RefreshInterval = 30 * time.Second
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.registry.AddRepo(ctx, cfg); err != nil {
		return err
	}

	if err := m.git.CloneBlobless(ctx, cfg); err != nil {
		return fmt.Errorf("clone blobless: %w", err)
	}

	headOID, headRef, err := m.git.ResolveHEAD(ctx, cfg)
	if err != nil {
		return fmt.Errorf("resolve HEAD: %w", err)
	}

	snap, err := snapshot.New(ctx, cfg.MetaDBPath)
	if err != nil {
		return fmt.Errorf("snapshot open: %w", err)
	}
	defer snap.Close()

	storedOID, _, gen, err := snap.ReadState(ctx)
	if err != nil || gen == 0 || storedOID != headOID {
		nodes, err := m.git.BuildTreeIndex(ctx, cfg, headOID)
		if err != nil {
			return fmt.Errorf("build tree index: %w", err)
		}
		if _, err := snap.PublishGeneration(ctx, headOID, headRef, nodes); err != nil {
			return fmt.Errorf("publish generation: %w", err)
		}
	}
	return nil
}

func (m *mountManager) PublishVolume(ctx context.Context, volumeID string, targetPath string, cfg model.RepoConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		return fmt.Errorf("mkdir target: %w", err)
	}

	repo, ok := m.repos[cfg.ID]
	if !ok {
		repo = &csiRepo{
			cfg:    cfg,
			refcnt: 0,
			mounts: map[string]fusefs.MountedFS{},
		}

		snap, err := snapshot.New(ctx, cfg.MetaDBPath)
		if err != nil {
			return fmt.Errorf("snapshot open: %w", err)
		}
		repo.snapshot = snap

		headOID, _, gen, err := snap.ReadState(ctx)
		if err != nil {
			snap.Close()
			return fmt.Errorf("read snapshot state: %w", err)
		}

		ov, err := overlay.New(ctx, cfg)
		if err != nil {
			snap.Close()
			return fmt.Errorf("overlay create: %w", err)
		}
		repo.overlay = ov

		h := hydrator.New(m.git)
		h.Start(4, cfg)
		repo.hydrator = h

		repo.resolver = &fusefs.Resolver{Snapshot: snap, Overlay: ov}
		repo.resolver.SetGeneration(gen)
		if ts, err := m.git.CommitTimestamp(ctx, cfg, headOID); err == nil {
			repo.resolver.SetCommitTime(ts)
		}

		repo.engine = &fusefs.Engine{
			Resolver: repo.resolver,
			Repo:     cfg,
			Overlay:  ov,
			Hydrator: h,
		}

		m.repos[cfg.ID] = repo
	}

	// Mount FUSE at the pod's target path
	mountCfg := cfg
	mountCfg.MountPath = targetPath

	mfs, err := fusefs.MountRepo(mountCfg, repo.resolver, repo.engine)
	if err != nil {
		return fmt.Errorf("fuse mount at %s: %w", targetPath, err)
	}
	go func() {
		_ = mfs.Join(context.Background())
	}()

	repo.mounts[targetPath] = mfs
	repo.refcnt++
	m.logger.Info("volume published", "volumeID", volumeID, "targetPath", targetPath)
	return nil
}

func (m *mountManager) UnpublishVolume(ctx context.Context, targetPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, repo := range m.repos {
		if mfs, ok := repo.mounts[targetPath]; ok {
			_ = mfs.Unmount()
			delete(repo.mounts, targetPath)
			repo.refcnt--
			if repo.refcnt <= 0 {
				repo.hydrator.Stop()
				repo.snapshot.Close()
				repo.overlay.Close()
				delete(m.repos, id)
			}
			m.logger.Info("volume unpublished", "targetPath", targetPath)
			return nil
		}
	}
	return fmt.Errorf("no mount found at %s", targetPath)
}

func (m *mountManager) DeleteRepo(ctx context.Context, volumeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, err := m.registry.GetRepo(ctx, volumeID)
	if err != nil {
		return err
	}

	if repo, ok := m.repos[cfg.ID]; ok {
		for targetPath, mfs := range repo.mounts {
			_ = mfs.Unmount()
			delete(repo.mounts, targetPath)
		}
		repo.hydrator.Stop()
		repo.snapshot.Close()
		repo.overlay.Close()
		delete(m.repos, cfg.ID)
	}

	if err := m.registry.RemoveRepo(ctx, volumeID); err != nil {
		return err
	}

	os.RemoveAll(cfg.GitDir)
	os.RemoveAll(cfg.OverlayDir)
	os.RemoveAll(cfg.BlobCacheDir)
	os.Remove(cfg.MetaDBPath)

	return nil
}
