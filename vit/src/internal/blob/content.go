package blob

import (
	"fmt"
	"os"
	"strings"
	"time"

	"vit/internal/fsutil"
	"vit/internal/types"
)

// genContentPath: absolute path to content node
func genContentPath(repoPath, assetPath string) string {
	return fsutil.PathOS(repoPath, ".vit", "content", assetPath)
}

// writeContentNodesRecc: write all content nodes to disk from a root node.
func writeContentNodesRecc(contentRootPath string, node *types.ContentNode) error {
	nodePath := fsutil.PathOS(contentRootPath, node.ContentHash[:2], node.ContentHash[2:])

	if _, err := os.Stat(nodePath); err == nil {
		// since we using content hash, if file exist at nodePath its content
		// (and its recc sub tree) are also identical
		return nil
	}

	if err := fsutil.WriteFileAtomic(nodePath, 0o644, func(f *os.File) error {
		_, err := f.Write(node.Serialized) // Assuming it has been serialized first!
		return err
	}); err != nil {
		return newErrContentNode(err, "failed to write content node", nodePath)
	}

	for _, c := range node.Children {
		if err := writeContentNodesRecc(contentRootPath, c); err != nil {
			return err
		}
	}

	return nil
}

func LoadContentTree(repoPath, assetPath, rootHash string) (*types.ContentNode, error) {
	return loadContentTree(genContentPath(repoPath, assetPath), rootHash)
}

func loadContentTree(contentRootPath, hash string) (*types.ContentNode, error) {
	nodePath := fsutil.PathOS(contentRootPath, hash[:2], hash[2:])

	b, err := os.ReadFile(nodePath)
	if err != nil {
		return nil, newErrContentNode(err, "failed to read content node", nodePath)
	}

	node, err := types.DeserializeContentNode(b)
	if err != nil {
		return nil, newErrContentNode(err, "failed to parse content node", nodePath)
	}
	node.ContentHash = hash

	for _, child := range node.Children {
		sub, err := loadContentTree(contentRootPath, child.ContentHash)
		if err != nil {
			return nil, err
		}
		child.Children = sub.Children
		child.Files = sub.Files
		child.Serialized = sub.Serialized
	}

	return node, nil
}

// PlanBranchFromContentNode plans copying a commit's content  into a new,
// writable branch working dir.
func PlanBranchFromContentNode(
	repoPath, assetPath, branchName string,
	node *types.ContentNode,
) (*types.BlobUpdatePlan, error) {
	return &types.BlobUpdatePlan{
		CopyRequests: []types.CopyFilesRequest{{
			DstRoot:  branchBlobPath(repoPath, assetPath, branchName),
			Src:      ContentNodeToDirStatSlice(node, fsutil.PathOS(repoPath, ".vit", "blob")),
			Writable: true,
		}},
	}, nil
}

func PlanEmptyBranch(
	repoPath, assetPath, branchName string,
) (*types.BlobUpdatePlan, *types.ContentNode, error) {
	branchPath := branchBlobPath(repoPath, assetPath, branchName)
	blobRootDir := fsutil.PathOS(repoPath, ".vit", "blob")

	empty := types.NewDirStat("")
	node, err := buildContentTreeFromDirStat(empty, branchPath, blobRootDir)
	if err != nil {
		return nil, nil, err
	}

	return &types.BlobUpdatePlan{
		CopyRequests: []types.CopyFilesRequest{{
			DstRoot:  branchPath,
			Src:      []*types.DirStat{empty},
			Writable: true,
		}},
		ContentRoots: []types.ContentRootWrite{{
			ContentRootPath: genContentPath(repoPath, assetPath),
			Root:            node,
		}},
	}, node, nil
}

// ContentNodeToDirStat flattens a content node into ONE DirStat keyed by
// tree-relative path, rooted at fullPath.
func ContentNodeToDirStat(n *types.ContentNode, fullPath string) *types.DirStat {
	ret := types.NewDirStat(fullPath)
	walkContentFiles(n, "", func(treeRel string, f *types.ContentNodeFile) {
		ret.Data[treeRel] = types.FileStat{Size: f.Size, MTime: f.MTime}
	})
	return ret
}

// ContentNodeToDirStatSlice groups a content node's files by the SOURCE ROOT their
// content lives under (one DirStat per root, keyed by tree path), relative to
// blobRoot.
func ContentNodeToDirStatSlice(n *types.ContentNode, blobRoot string) []*types.DirStat {
	byRoot := map[string]*types.DirStat{}
	walkContentFiles(n, "", func(treeRel string, f *types.ContentNodeFile) {
		rootRel := strings.TrimSuffix(f.BlobTarget, treeRel) // BlobTarget == <root>/<treeRel>
		rootAbs := fsutil.PathOS(blobRoot, rootRel)
		ds := byRoot[rootAbs]
		if ds == nil {
			ds = types.NewDirStat(rootAbs)
			byRoot[rootAbs] = ds
		}
		ds.Data[treeRel] = types.FileStat{Size: f.Size, MTime: f.MTime}
	})

	out := make([]*types.DirStat, 0, len(byRoot))
	for _, ds := range byRoot {
		out = append(out, ds)
	}
	return out
}

func walkContentFiles(n *types.ContentNode, prefix string, fn func(string, *types.ContentNodeFile)) {
	for _, f := range n.Files {
		fn(fsutil.PathInternal(prefix, f.Name), f)
	}
	for _, c := range n.Children {
		walkContentFiles(c, fsutil.PathInternal(prefix, c.Name), fn)
	}
}

// buildContentTreeFromDirStat: build a tree of content nodes from a dir stat object,
// return the root node.
func buildContentTreeFromDirStat(srcDirStat *types.DirStat, commitDir, blobRootDir string) (*types.ContentNode, error) {
	root := &types.ContentNode{Name: fsutil.BaseInternal(commitDir)}

	for relPath, stat := range srcDirStat.Data {
		blobTarget, err := fsutil.RelInternal(blobRootDir, fsutil.PathInternal(commitDir, relPath))
		if err != nil {
			return nil, err
		}

		// DirStat keys are POSIX- PathIternal() paths, so split on "/".
		parts := strings.Split(relPath, "/")
		node := root
		for _, dir := range parts[:len(parts)-1] {
			node = ensureChildDir(node, dir)
		}
		node.Files = append(node.Files, &types.ContentNodeFile{
			Name:       parts[len(parts)-1],
			BlobTarget: blobTarget,
			Size:       stat.Size,
			MTime:      stat.MTime,
		})
	}

	hashContentTree(root)
	return root, nil
}

func ensureChildDir(parent *types.ContentNode, name string) *types.ContentNode {
	for _, c := range parent.Children {
		if c.Name == name {
			return c
		}
	}
	child := &types.ContentNode{Name: name}
	parent.Children = append(parent.Children, child)
	return child
}

func hashContentTree(node *types.ContentNode) {
	for _, c := range node.Children {
		hashContentTree(c)
	}
	node.Serialized = node.Serialize()
	node.ContentHash = fsutil.HashBytes(node.Serialized)
}

// applyChangeRecord: modify a tree of content node according a ChangeRecord object
func applyChangeRecord(
	n *types.ContentNode,
	cr *types.ChangeRecord,
	baseline time.Time,
	commitDir, blobRootDir string,
) error {
	plan := applyPlan{
		fileChanges: make(map[string][]contentChange),
		dirsTouched: make(map[string]map[string]bool),
	}

	for _, f := range cr.Files {
		dir := fsutil.DirInternal(f.Path)

		blobTarget, err := fsutil.RelInternal(blobRootDir, fsutil.PathInternal(commitDir, f.Path))
		if err != nil {
			return err
		}
		plan.fileChanges[dir] = append(plan.fileChanges[dir], contentChange{
			name:       fsutil.BaseInternal(f.Path),
			typ:        f.Type,
			size:       f.Size,
			blobTarget: blobTarget,
		})

		curr := ""
		for _, sub := range fsutil.SplitInternal(dir) {
			curr = fsutil.PathInternal(curr, sub)
			parent := fsutil.DirInternal(curr)
			if plan.dirsTouched[parent] == nil {
				plan.dirsTouched[parent] = make(map[string]bool)
			}
			plan.dirsTouched[parent][fsutil.BaseInternal(curr)] = true
		}
	}

	applyPlanRecc(n, &plan, ".", baseline)
	hashContentTree(n)
	return nil
}

type applyPlan struct {
	fileChanges map[string][]contentChange
	dirsTouched map[string]map[string]bool
}

type contentChange struct {
	name       string
	typ        types.FileChangeType
	size       int64
	blobTarget string
}

func applyPlanRecc(n *types.ContentNode, plan *applyPlan, nodePath string, baseline time.Time) {
	for _, fc := range plan.fileChanges[nodePath] {
		switch fc.typ {
		case types.FileChangeAdded:
			n.Files = append(n.Files, &types.ContentNodeFile{
				Name:       fc.name,
				Size:       fc.size,
				MTime:      baseline,
				BlobTarget: fc.blobTarget,
			})
		case types.FileChangeEdited:
			for _, f := range n.Files {
				if f.Name == fc.name {
					f.Size = fc.size
					f.MTime = baseline
					f.BlobTarget = fc.blobTarget
					break
				}
			}
		case types.FileChangeRemoved:
			for i, f := range n.Files {
				if f.Name == fc.name {
					n.Files = append(n.Files[:i], n.Files[i+1:]...)
					break
				}
			}
		}
	}

	for childName := range plan.dirsTouched[nodePath] {
		var child *types.ContentNode
		for _, sub := range n.Children {
			if sub.Name == childName {
				child = sub
				break
			}
		}
		if child == nil {
			child = &types.ContentNode{Name: childName}
			n.Children = append(n.Children, child)
		}
		applyPlanRecc(child, plan, fsutil.PathInternal(nodePath, childName), baseline)
	}
}

func newErrContentNode(nestedErr error, msg, nodePath string) error {
	return types.NewInternalVitError(
		types.ErrBlobInternal,
		nestedErr,
		[]string{fmt.Sprintf("%s: %s", msg, nodePath)},
		[]any{"nodePath", nodePath},
	)
}
