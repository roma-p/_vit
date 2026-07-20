package blob

import (
	"fmt"
	"os"
	"strings"

	"vit/internal/fsutil"
	"vit/internal/types"
)

func genContentPath(repoPath, assetPath string) string {
	return fsutil.PathOS(repoPath, ".vit", "content", assetPath)
}

func newErrContentNode(nestedErr error, msg, nodePath string) error {
	return types.NewInternalVitError(
		types.ErrBlobInternal,
		nestedErr,
		[]string{fmt.Sprintf("%s: %s", msg, nodePath)},
		[]any{"nodePath", nodePath},
	)
}

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
