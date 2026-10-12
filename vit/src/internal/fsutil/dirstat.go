package fsutil

import (
	"context"
	"io/fs"
	"path/filepath"
	"slices"

	"vit/internal/types"
)

// NewDirStatFromDir walk given dir to build a DirStat object from it.
// DirStat only tracks files, not dir (like git)
//
// skipRootDirs names direct children of path to skip entirely
// (to not track dependencies)
func NewDirStatFromDir(ctx context.Context, path string, skipRootDirs ...string) (*types.DirStat, error) {
	ret := types.NewDirStat(path)

	path = PathOS(path)

	if err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if len(skipRootDirs) > 0 {
			if rel, rerr := RelInternal(path, p); rerr == nil && slices.Contains(skipRootDirs, rel) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		rel, err := RelInternal(path, p) // path stored in FileStat are always internal!
		if err != nil {
			return err
		}

		ret.Data[rel] = types.FileStat{
			Size:  info.Size(),
			MTime: info.ModTime(),
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return ret, nil
}
