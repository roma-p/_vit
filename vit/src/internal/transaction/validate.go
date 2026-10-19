package transaction

import (
	"fmt"
	"os"
	"strings"

	"vit/internal/fsutil"
	"vit/internal/types"
)

// scanForConflicts checks every tid against the in-progress transactions of its OWN
// repo (tids are grouped by repo, so a multi-repo transaction is validated across
// all involved repos). Conflict rules, unchanged:
//   - a TreeChanged tid conflicts with ANY overlapping transaction;
//   - an edit (Commit/Branch/Tag) conflicts with an overlapping TreeChanged, or
//     with another edit on the SAME object.
func scanForConflicts(tids []TransactionID) error {
	byRepo := map[string][]TransactionID{}
	for i := range tids {
		p := tids[i].Ref.RepoPath
		byRepo[p] = append(byRepo[p], tids[i])
	}

	for repoPath, group := range byRepo {
		existing, err := scanExistingTransactions(repoPath)
		if err != nil {
			return err
		}
		for _, t := range group {
			for i := range existing {
				e := &existing[i]
				if e.Ref == nil || !pathsOverlap(t.Ref.AssetPath, e.Ref.AssetPath) {
					continue
				}
				if t.Operation == TransactionTreeChanged || e.Operation == TransactionTreeChanged {
					return newConflictErrorFromRef(t.Ref, e)
				}
				if t.Ref.AssetPath == e.Ref.AssetPath && t.Ref.ObjectPath() == e.Ref.ObjectPath() {
					return newConflictErrorFromRef(t.Ref, e)
				}
			}
		}
	}
	return nil
}

// -- FILE LOCAL -------------------------------------------------------------

func scanExistingTransactions(repoPath string) ([]TransactionID, error) {
	transactionDir := fsutil.PathOS(repoPath, ".vit", "transaction")

	entries, err := os.ReadDir(transactionDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, types.NewInternalVitError(
			types.ErrTransactionFailed, err,
			[]string{"failed to read transaction directory"},
			nil,
		)
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
