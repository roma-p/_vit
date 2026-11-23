package db

import (
	"context"
	"os"
	"slices"

	"vit/internal/fsutil"
	"vit/internal/history"
	"vit/internal/types"
)

// AttrIndex is the attribute index of one repo, held in memory and loaded lazily, key by key.
// build: the build marker (_build.json), read once; nil until the first get.
// shards: the shards already loaded, by key; a CLI command drops them on exit, a session keeps them.
type AttrIndex struct {
	repoPath string
	build    *attrIndexBuild
	shards   map[string]*attrShard
}

// attrShard holds all assets uiids which have a common attributes, sharded by their other attribute.
// Cursor : last up to date.
// Values format is map <other key>: [<list of all uids which have this parm.>]
type attrShard struct {
	Cursor history.Cursor      `json:"cursor"`
	Values map[string][]string `json:"values"`
}

type attrIndexBuild struct {
	Cursor history.Cursor `json:"cursor"`
}

func NewAttrIndex(repoPath string) *AttrIndex {
	return &AttrIndex{repoPath: repoPath, shards: map[string]*attrShard{}}
}

// get resolve shards for given set of attr keys.
func (i *AttrIndex) get(ctx context.Context, keys []string) (map[string]*attrShard, error) {
	if i.build == nil {
		build, err := readAttrIndexFile[attrIndexBuild](pathAttrIndexBuild(i.repoPath))
		if err != nil {
			return nil, err
		}
		if build == nil {
			if build, err = buildAttrIndex(ctx, i.repoPath); err != nil {
				return nil, err
			}
		}
		i.build = build
	}
	out := make(map[string]*attrShard, len(keys))
	for _, k := range keys {
		s := i.shards[k]
		if s == nil {
			var err error
			if s, err = readAttrIndexFile[attrShard](pathAttrShard(i.repoPath, k)); err != nil {
				return nil, err
			}
			if s == nil {
				s = &attrShard{Cursor: i.build.Cursor, Values: map[string][]string{}}
			}
			i.shards[k] = s
		}
		out[k] = s
	}
	return out, nil
}

// -- FILE LOCAL -------------------------------------------------------------

func buildAttrIndex(ctx context.Context, repoPath string) (*attrIndexBuild, error) {
	stop, err := fsutil.AcquireExclusiveLockWithKeepalive(ctx, pathAttrIndexRebuild(repoPath))
	if err != nil {
		return nil, err
	}
	defer stop()
	if build, err := readAttrIndexFile[attrIndexBuild](pathAttrIndexBuild(repoPath)); err != nil || build != nil {
		return build, err
	}

	cursor, err := history.Head(repoPath)
	if err != nil {
		return nil, newErrDBInternal(err, "failed to read the history head")
	}
	nodePaths, err := scanAllTreeNodeFiles(repoPath)
	if err != nil {
		return nil, err
	}
	shards := map[string]*attrShard{}
	for _, p := range nodePaths {
		h, err := fsutil.NewJSONHandlerFromFile[types.TreeNode](p)
		if err != nil {
			return nil, newErrDBInternal(err, "failed to read tree node "+p)
		}
		if h.Data.Type != types.TreeNodeTypeAsset {
			continue
		}
		for k, v := range h.Data.LiveAttrs() {
			if shards[k] == nil {
				shards[k] = &attrShard{Cursor: cursor, Values: map[string][]string{}}
			}
			shards[k].Values[v.Value] = append(shards[k].Values[v.Value], h.Data.ID)
		}
	}

	for k, s := range shards {
		for _, uids := range s.Values {
			slices.Sort(uids)
		}
		if err := fsutil.NewJSONHandlerFromPath(pathAttrShard(repoPath, k), s).Write(); err != nil {
			return nil, newErrDBInternal(err, "failed to write attribute shard "+k)
		}
	}
	build := &attrIndexBuild{Cursor: cursor}
	if err := fsutil.NewJSONHandlerFromPath(pathAttrIndexBuild(repoPath), build).Write(); err != nil {
		return nil, newErrDBInternal(err, "failed to write the attribute index build")
	}
	return build, nil
}

func readAttrIndexFile[T any](path string) (*T, error) {
	h, err := fsutil.NewJSONHandlerFromFile[T](path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, newErrDBInternal(err, "failed to read "+path)
	}
	return h.Data, nil
}

func pathAttrIndex(repoPath string) string {
	return fsutil.PathOS(repoPath, ".vit", "cache", "attrindex")
}

func pathAttrShard(repoPath, key string) string {
	return fsutil.PathOS(pathAttrIndex(repoPath), key+".json")
}

func pathAttrIndexBuild(repoPath string) string {
	return fsutil.PathOS(pathAttrIndex(repoPath), "_build.json")
}

func pathAttrIndexRebuild(repoPath string) string {
	return fsutil.PathOS(repoPath, ".vit", "cache", ".attrindex_rebuild")
}
