package vit

import (
	"context"
	"log/slog"

	"vit/internal/config"
	"vit/internal/types"
)

func registerCrossRepo(ctx context.Context, logger *slog.Logger, a, b *types.Ref) {
	if a.RepoUID == b.RepoUID {
		return
	}
	if err := config.RegisterRepo(ctx, a.RepoPath, b.RepoUID, b.RepoPath); err != nil {
		logger.Warn("cross-repo: register mapping failed", "in", a.RepoPath, "uid", b.RepoUID, "err", err)
	}
	if err := config.RegisterRepo(ctx, b.RepoPath, a.RepoUID, a.RepoPath); err != nil {
		logger.Warn("cross-repo: register mapping failed", "in", b.RepoPath, "uid", a.RepoUID, "err", err)
	}
}
