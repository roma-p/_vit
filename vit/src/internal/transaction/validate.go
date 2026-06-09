package transaction

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"vit/internal/types"
)

// validateTreeChanged checks that no existing transaction conflicts with
// a tree mutation (add/move/delete). TreeChanged is exclusive: it conflicts
// with ANY existing transaction on overlapping paths.
func validateTreeChanged(repoPath string, assetPaths []string) error {
	existing, err := scanExistingTransactions(repoPath)
	if err != nil {
		return err
	}

	for _, path := range assetPaths {
		for i := range existing {
			if pathsOverlap(path, existing[i].Ref.AssetPath) {
				return newConflictError(path, &existing[i])
			}
		}
	}
	return nil
}

// validateEdit checks that no existing transaction conflicts with a
// commit or branch operation. Edit operations conflict with:
//   - Any TreeChanged on overlapping paths
//   - Another Commit/Branch targeting the same ref
func validateEdit(repoPath string, refs []*types.Ref) error {
	existing, err := scanExistingTransactions(repoPath)
	if err != nil {
		return err
	}

	for _, ref := range refs {
		for i := range existing {
			e := &existing[i]
			if !pathsOverlap(ref.AssetPath, e.Ref.AssetPath) {
				continue
			}
			if e.Operation == TransactionTreeChanged {
				return newConflictErrorFromRef(ref, e)
			}
			if ref.AssetPath == e.Ref.AssetPath && ref.ObjectPath() == e.Ref.ObjectPath() {
				return newConflictErrorFromRef(ref, e)
			}
		}
	}
	return nil
}

func scanExistingTransactions(repoPath string) ([]TransactionID, error) {
	transactionDir := filepath.Join(repoPath, ".vit", "transaction")

	entries, err := os.ReadDir(transactionDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read transaction directory: %w", err)
	}

	var tids []TransactionID
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		tid, err := newTransactionIDFromName(repoPath, entry.Name())
		if err != nil {
			continue
		}
		tids = append(tids, *tid)
	}
	return tids, nil
}

// pathsOverlap returns true if a is a prefix of b, b is a prefix of a,
// or they are equal. Prefix checks use "/" boundary to avoid
// "foo/bar" matching "foo/barbaz".
func pathsOverlap(a, b string) bool {
	if a == b {
		return true
	}
	return strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

func newConflictError(assetPath string, existing *TransactionID) error {
	return types.NewVitError(
		types.ErrTransactionConflict,
		[]string{
			fmt.Sprintf("conflict with in-progress operation (%s) on: %s",
				existing.Operation, existing.Ref.AssetPath),
		},
		[]any{
			"assetPath", assetPath,
			"existingOperation", existing.Operation,
			"existingAssetPath", existing.Ref.AssetPath,
		},
	)
}

func newConflictErrorFromRef(ref *types.Ref, existing *TransactionID) error {
	return types.NewVitError(
		types.ErrTransactionConflict,
		[]string{
			fmt.Sprintf("conflict with in-progress operation (%s) on: %s",
				existing.Operation, existing.Ref.AssetPath),
		},
		[]any{
			"ref", ref.RelativePath(),
			"existingOperation", existing.Operation,
			"existingAssetPath", existing.Ref.AssetPath,
		},
	)
}
