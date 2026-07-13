package blob

import (
	"fmt"
	"os"

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

func initAssetContentBlob(repoPath, assetPath, blobDir string) (*types.ContentNode, error) {
	rootDir := fsutil.PathOS(repoPath, ".vit", "blob")
	rootContentNode, err := buildInMemoryContentTree(rootDir, blobDir)
	if err != nil {
		return nil, err
	}

	contentRootPath := genContentPath(repoPath, assetPath)

	if err := writeContentNodesRecc(contentRootPath, rootContentNode); err != nil {
		return nil, err
	}

	return rootContentNode, nil
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

	node, err := types.ParseContentNode(b)
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

func buildInMemoryContentTree(rootdir, currDir string) (*types.ContentNode, error) {
	entries, err := os.ReadDir(currDir)
	if err != nil {
		return nil, err
	}

	node := types.ContentNode{Name: fsutil.BaseInternal(currDir)}

	for _, e := range entries {
		subPath := fsutil.PathOS(currDir, e.Name())
		if e.IsDir() {
			subNode, err := buildInMemoryContentTree(rootdir, subPath)
			if err != nil {
				return nil, err
			}
			node.Children = append(node.Children, subNode)
		} else {
			info, err := e.Info()
			if err != nil {
				return nil, err
			}

			blobTarget, err := fsutil.RelInternal(
				rootdir,
				fsutil.PathInternal(currDir, e.Name()),
			)
			if err != nil {
				return nil, err
			}

			node.Files = append(node.Files, &types.ContentNodeFile{
				Name:       e.Name(),
				BlobTarget: blobTarget,
				Size:       info.Size(),
				MTime:      info.ModTime(),
			})
		}
	}

	node.Serialized = node.Serialize()
	node.ContentHash = fsutil.HashBytes(node.Serialized)
	return &node, nil
}
