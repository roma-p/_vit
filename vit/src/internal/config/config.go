package config

import (
	"context"
	"fmt"
	"os"

	"vit/internal/fsutil"
	"vit/internal/types"
)

type RepoConfig struct {
	CurrentSite string            `json:"current_site"`
	Sites       map[string]string `json:"sites"` // SiteUID → alias

	// used for cross repository work.
	RepoUID  string            `json:"repo_uid"`
	RepoPath string            `json:"-"`
	Repos    map[string]string `json:"repos"` // repoUID → local path
}

func LoadRepoConfig(repoPath string) (*RepoConfig, error) {
	if !fsutil.CheckPathIsVitRepo(repoPath) {
		panic("expected root repo path")
	}

	configPath := genRepoConfigPath(repoPath)
	if _, err := os.Stat(configPath); err != nil {
		return nil, newMissingConfigPathError(configPath, err)
	}

	jsonData, err := fsutil.NewJSONHandlerFromFile[RepoConfig](configPath)
	ret := jsonData.Data
	ret.RepoPath = repoPath
	if err != nil {
		return nil, newConfigReadError(configPath, err)
	}
	return ret, nil
}

func InitRepoConfig(repoPath, currentSiteAlias string) error {
	// TODO: asset repoPath is root .vit
	curSiteUID := fsutil.GenerateUID(8)
	if currentSiteAlias == "" {
		currentSiteAlias = curSiteUID
	}

	repoUID := fsutil.GenerateUID(8)

	jsonAsset := fsutil.NewJSONHandlerFromPath(
		genRepoConfigPath(repoPath),
		&RepoConfig{
			Sites:       make(map[string]string),
			CurrentSite: curSiteUID,
			RepoUID:     repoUID,
			Repos:       make(map[string]string),
		},
	)
	jsonAsset.Data.Sites[curSiteUID] = currentSiteAlias
	if err := jsonAsset.Write(); err != nil {
		return newConfigWriteError(genRepoConfigPath(repoPath), err)
	}
	return nil
}

func RegisterRepo(ctx context.Context, repoPath, uid, localPath string) error {
	configPath := genRepoConfigPath(repoPath)
	return fsutil.WithExclusiveLock(ctx, configPath, func() error {
		cfg, err := LoadRepoConfig(repoPath)
		if err != nil {
			return err
		}
		if cfg.Repos == nil {
			cfg.Repos = make(map[string]string)
		}
		if cfg.Repos[uid] == localPath {
			return nil
		}
		cfg.Repos[uid] = localPath
		return fsutil.NewJSONHandlerFromPath(configPath, cfg).Write()
	})
}

func genRepoConfigPath(repoPath string) string {
	return fsutil.PathOS(repoPath, ".vit", "config.json")
}

func newMissingConfigPathError(configPath string, err error) error {
	return types.NewInternalVitError(
		types.ErrInternalRepoCorrupted,
		err,
		[]string{fmt.Sprintf("repository config not found at: %s", configPath)},
		[]any{"MissingFile", configPath},
	)
}

func newConfigReadError(configPath string, err error) error {
	return types.NewInternalVitError(
		types.ErrInternalRepoCorrupted,
		err,
		[]string{fmt.Sprintf("failed to read repository config at: %s", configPath)},
		[]any{"configPath", configPath},
	)
}

func newConfigWriteError(configPath string, err error) error {
	return types.NewInternalVitError(
		types.ErrInternalRepoCorrupted,
		err,
		[]string{fmt.Sprintf("failed to write repository config at: %s", configPath)},
		[]any{"configPath", configPath},
	)
}
