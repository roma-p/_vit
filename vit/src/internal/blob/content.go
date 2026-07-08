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

func initAssetContentBlob(repoPath, assetPath, blobDir string) (string, error) {
	rootDir := fsutil.PathOS(repoPath, ".vit", "blob")
	rootContentNode, err := buildInMemoryContentTree(rootDir, blobDir)
	if err != nil {
		return "", err
	}

	contentRootPath := genContentPath(repoPath, assetPath)

	if err := writeContentNodesRecc(contentRootPath, rootContentNode); err != nil {
		return "", err
	}

	return rootContentNode.UID, nil
}

func writeContentNodesRecc(contentRootPath string, node *types.ContentNode) error {
	str := node.Serialize()

	nodePath := fsutil.PathOS(contentRootPath, node.UID[:2], node.UID[2:])

	// WTF? Simplify this WriteFileAtomic...
	if err := fsutil.WriteFileAtomic(nodePath, 0o644, func(f *os.File) error {
		_, err := f.Write(str)
		return err
	}); err != nil {
		return fmt.Errorf("content: write node %s", err)
	}

	for _, c := range node.Children {
		if err := writeContentNodesRecc(contentRootPath, c); err != nil {
			return err
		}
	}

	return nil
}

// NO! Name is not enough. We need to track diff i guess
func buildInMemoryContentTree(rootdir, currDir string) (*types.ContentNode, error) {
	entries, err := os.ReadDir(currDir)
	if err != nil {
		return nil, err
	}

	node := types.ContentNode{
		Name: fsutil.BaseInternal(currDir),
		UID:  fsutil.GenerateUID(8),
	}

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
	return &node, nil
}
