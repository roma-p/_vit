package db

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"slices"
	"strings"

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

// attrShard is the inverted index of one attribute key.
// Cursor: the history position the shard is up to date at.
// Values: value of that key -> sorted uids of the assets having it.
type attrShard struct {
	Cursor history.Cursor      `json:"cursor"`
	Values map[string][]string `json:"values"`
}

type attrIndexBuild struct {
	Cursor history.Cursor `json:"cursor"`
	Format int            `json:"format"`
}

const attrIndexFormat = 1

type attrShardFile struct {
	Cursor history.Cursor `json:"cursor"`
	Values [][]string     `json:"values"`
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
		if build == nil || build.Format != attrIndexFormat {
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
			if s, err = readAttrShard(i.repoPath, k); err != nil {
				return nil, err
			}
			if s == nil {
				s = &attrShard{Cursor: i.build.Cursor, Values: map[string][]string{}}
			}
			i.shards[k] = s
		}
		out[k] = s
	}
	if err := i.refreshShardsFromHistory(out); err != nil {
		return nil, err
	}
	return out, nil
}

// -- FILE LOCAL -------------------------------------------------------------

func (i *AttrIndex) refreshShardsFromHistory(shards map[string]*attrShard) error {
	// 1. read the history once from the oldest cursor.
	entries, next, err := history.ReadSince(i.repoPath, oldestCursor(shards))
	if err != nil {
		return newErrDBInternal(err, "failed to read the history")
	}

	// 2. collect each touched uid with the asked keys it touches
	touched := map[string][]string{}
	for _, e := range entries {
		if len(e.NodeID) < 2 {
			continue
		}
		for _, c := range e.Attrs {
			if shards[c.Key] != nil && !slices.Contains(touched[e.NodeID], c.Key) {
				touched[e.NodeID] = append(touched[e.NodeID], c.Key)
			}
		}
	}

	// 3. read each relevant tree node
	for uid, keys := range touched {
		node, err := readAttrIndexFile[types.TreeNode](treeNodeJSONPath(i.repoPath, uid))
		if err != nil {
			return err
		}

		// a deleted asset (nil) or a removed key (tombstone) has no live value
		live := map[string]types.AttrValue{}
		if node != nil && node.Type == types.TreeNodeTypeAsset {
			live = node.LiveAttrs()
		}

		// 4. update with live value.
		for _, k := range keys {
			shards[k].drop(uid)
			if v, ok := live[k]; ok {
				shards[k].insert(v.Value, uid)
			}
		}
	}

	// 5. move every asked shard to the same cursor.
	for k, s := range shards {
		if maps.Equal(s.Cursor, next) {
			continue
		}
		s.Cursor = next
		storeAttrShard(i.repoPath, k, s)
	}
	return nil
}

func (s *attrShard) insert(value, uid string) {
	uids := s.Values[value]
	if at, found := slices.BinarySearch(uids, uid); !found {
		s.Values[value] = slices.Insert(uids, at, uid)
	}
}

func (s *attrShard) drop(uid string) {
	for v, uids := range s.Values {
		if at, found := slices.BinarySearch(uids, uid); found {
			if uids = slices.Delete(uids, at, at+1); len(uids) == 0 {
				delete(s.Values, v)
			} else {
				s.Values[v] = uids
			}
		}
	}
}

func oldestCursor(shards map[string]*attrShard) history.Cursor {
	sites := map[string]bool{}
	for _, s := range shards {
		for site := range s.Cursor {
			sites[site] = true
		}
	}
	out := history.Cursor{}
	for site := range sites {
		var oldest *history.Position
		for _, s := range shards {
			p := s.Cursor[site]
			if oldest == nil || p.Segment < oldest.Segment || p.Segment == oldest.Segment && p.Line < oldest.Line {
				oldest = &p
			}
		}
		out[site] = *oldest
	}
	return out
}

func storeAttrShard(repoPath, key string, s *attrShard) {
	_ = writeAttrShard(repoPath, key, s)
}

func buildAttrIndex(ctx context.Context, repoPath string) (*attrIndexBuild, error) {
	stop, err := fsutil.AcquireExclusiveLockWithKeepalive(ctx, pathAttrIndexRebuild(repoPath))
	if err != nil {
		return nil, newErrDBInternal(err, "failed to lock the attribute index rebuild")
	}
	defer stop()
	if build, err := readAttrIndexFile[attrIndexBuild](pathAttrIndexBuild(repoPath)); err != nil || build != nil && build.Format == attrIndexFormat {
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
		if err := writeAttrShard(repoPath, k, s); err != nil {
			return nil, newErrDBInternal(err, "failed to write attribute shard "+k)
		}
	}
	if err := removeStaleAttrShards(repoPath, shards); err != nil {
		return nil, err
	}
	build := &attrIndexBuild{Cursor: cursor, Format: attrIndexFormat}
	if err := fsutil.NewJSONHandlerFromPath(pathAttrIndexBuild(repoPath), build).Write(); err != nil {
		return nil, newErrDBInternal(err, "failed to write the attribute index build")
	}
	return build, nil
}

func removeStaleAttrShards(repoPath string, keep map[string]*attrShard) error {
	entries, err := os.ReadDir(pathAttrIndex(repoPath))
	if err != nil && !os.IsNotExist(err) {
		return newErrDBInternal(err, "failed to list the attribute index")
	}
	for _, e := range entries {
		key, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || keep[key] != nil || types.CheckAttrKey(key) != nil {
			continue
		}
		if err := os.Remove(pathAttrShard(repoPath, key)); err != nil && !os.IsNotExist(err) {
			return newErrDBInternal(err, "failed to remove attribute shard "+key)
		}
	}
	return nil
}

func readAttrShard(repoPath, key string) (*attrShard, error) {
	data, err := os.ReadFile(pathAttrShard(repoPath, key))
	if os.IsNotExist(err) {
		return nil, nil
	}
	var f attrShardFile
	if err == nil {
		err = json.Unmarshal(data, &f)
	}
	if err != nil {
		return nil, newErrDBInternal(err, "failed to read attribute shard "+key)
	}
	s := &attrShard{Cursor: f.Cursor, Values: make(map[string][]string, len(f.Values))}
	for _, row := range f.Values {
		if len(row) > 1 {
			s.Values[row[0]] = row[1:]
		}
	}
	return s, nil
}

func writeAttrShard(repoPath, key string, s *attrShard) error {
	f := attrShardFile{Cursor: s.Cursor, Values: make([][]string, 0, len(s.Values))}
	for _, v := range slices.Sorted(maps.Keys(s.Values)) {
		f.Values = append(f.Values, append([]string{v}, s.Values[v]...))
	}
	return fsutil.WriteFileAtomic(pathAttrShard(repoPath, key), 0o644, func(file *os.File) error {
		return json.NewEncoder(file).Encode(f)
	})
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
