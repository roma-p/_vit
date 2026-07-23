package config

import (
	"fmt"
	"os"

	"vit/internal/fsutil"
	"vit/internal/types"
)

type RepoConfig struct {
	Sites       map[string]string `json:"sites"` // SiteUID → alias
	CurrentSite string            `json:"current_site"`
}

func LoadRepoConfig(repoPath string) (*RepoConfig, error) {
	if !fsutil.CheckPathIsVitRepo(repoPath) {
		panic("expected root repo path")
	}

	configPath := genRepoConfigPath(repoPath)
	if _, err := os.Stat(configPath); err != nil {
		return nil, newMissingConfigPathError(configPath, err)
	}

	jsonAsset, err := fsutil.NewJSONHandlerFromFile[RepoConfig](configPath)
	if err != nil {
		return nil, newConfigReadError(configPath, err)
	}
	return jsonAsset.Data, nil
}

func InitRepoConfig(repoPath, currentSiteAlias string) error {
	// TODO: asset repoPath is root .vit
	curSiteUID := fsutil.GenerateUID(8)
	if currentSiteAlias == "" {
		currentSiteAlias = curSiteUID
	}
	jsonAsset := fsutil.NewJSONHandlerFromPath[RepoConfig](
		genRepoConfigPath(repoPath),
		&RepoConfig{
			Sites:       make(map[string]string),
			CurrentSite: curSiteUID,
		},
	)
	jsonAsset.Data.Sites[curSiteUID] = currentSiteAlias
	if err := jsonAsset.Write(); err != nil {
		return newConfigWriteError(genRepoConfigPath(repoPath), err)
	}
	return nil
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
