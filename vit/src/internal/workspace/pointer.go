package workspace

import (
	"context"
	"fmt"
	"os"
	"strings"

	"vit/internal/fsutil"
	"vit/internal/opcontext"
	"vit/internal/types"
)

// Pointer are objects that map worksapce -> reference
// reference -> workspace path is handled by workspace.pathgen
// but without poitner,  we would have to rely on path logic (like shotgrid sgtk)
// which is exactly what we are trying to avoid in vit
type Pointer struct {
	WorkspacePath string    `json:"workspace_path"` // relative to repo root
	Ref           types.Ref `json:"ref"`
}

type PointerJSON = fsutil.JSONHandler[Pointer]

func writePointer(
	ctx context.Context,
	pool *fsutil.JSONHandlerPool,
	repoPath, wsPath string,
	ref *types.Ref,
) error {
	rel, err := fsutil.RelInternal(repoPath, wsPath)
	if err != nil {
		return newErrWorkspaceInternal(err, "pointer: workspace path outside repo: "+wsPath)
	}

	h, err := fsutil.ResolveHandler(ctx, pool, pointerPath(repoPath, rel), true, &Pointer{
		WorkspacePath: rel,
		Ref:           *ref,
	})
	if err != nil {
		return newErrWorkspaceInternal(err, "pointer: could not open handler for "+rel)
	}

	h.Data = &Pointer{WorkspacePath: rel, Ref: *ref}
	if err := h.Write(); err != nil {
		return newErrWorkspaceInternal(err, "pointer: write failed for "+rel)
	}
	return nil
}

// ResolveRefFromPath resolves a user-given path to a ref: first as a ref string
// (@type=value), else by walking up from the workspace path to the nearest pointer.
func ResolveRefFromPath(
	ctx context.Context,
	opctx *opcontext.OperationContext,
	path string,
) (*types.Ref, error) {
	repoPath := opctx.RepoPath()
	if ref, err := types.NewRefFromPath(repoPath, path); err == nil {
		uid, err := opctx.ResolveRepoUIDFromPath(repoPath)
		if err != nil {
			return nil, err
		}
		ref.RepoUID = uid
		if ref.RefType == types.RefTypeTag && ref.Site == "" {
			ref.Site = opctx.RepoRefFor(uid).Site
		}
		return ref, nil
	}
	for cur := fsutil.PathInternal(path); cur != "." && cur != "/" && cur != ""; cur = fsutil.DirInternal(cur) {
		pp := pointerPath(repoPath, cur)
		if _, err := os.Stat(pp); err != nil {
			continue
		}
		h, err := fsutil.ResolveHandler[Pointer](ctx, opctx.JSONPool, pp, false, nil)
		if err != nil {
			return nil, NewErrUntrackedFile(path)
		}
		ref := h.Data.Ref
		ref.RepoPath = opctx.RepoRefFor(ref.RepoUID).Path
		return &ref, nil
	}
	return nil, NewErrUntrackedFile(path)
}

// ResolveWorkspacePathFromRef finds where a ref is materialized by scanning its
// asset's pointer files and matching the recorded ref.
func ResolveWorkspacePathFromRef(repoPath string, ref *types.Ref) (string, bool, error) {
	var kind string
	switch ref.RefType {
	case types.RefTypeBranch:
		kind = "branches"
	case types.RefTypeCommit:
		kind = "snapshots"
	case types.RefTypeTag:
		kind = "tags"
	default:
		return "", false, nil
	}
	base := fsutil.PathOS(repoPath, ".vit", "workspace", "pointers",
		fsutil.PathInternal(ref.AssetPath, kind))
	return scanPointersForRef(base, ref)
}

func scanPointersForRef(dir string, ref *types.Ref) (string, bool, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, newErrWorkspaceInternal(err, "pointer: scan failed for "+dir)
	}
	for _, e := range entries {
		full := fsutil.PathOS(dir, e.Name())
		if e.IsDir() { // tags nest by family
			if p, ok, err := scanPointersForRef(full, ref); err != nil || ok {
				return p, ok, err
			}
			continue
		}
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		h, err := fsutil.NewJSONHandlerFromFile[Pointer](full)
		if err != nil {
			continue // tolerate a corrupt/partial pointer
		}
		if sameObject(&h.Data.Ref, ref) {
			return h.Data.WorkspacePath, true, nil
		}
	}
	return "", false, nil
}

func RemovePointer(repoPath, wsPath string) error {
	rel, err := fsutil.RelInternal(repoPath, wsPath)
	if err != nil {
		return newErrWorkspaceInternal(err, "pointer: workspace path outside repo: "+wsPath)
	}
	if err := os.Remove(pointerPath(repoPath, rel)); err != nil && !os.IsNotExist(err) {
		return newErrWorkspaceInternal(err, "pointer: remove failed for "+rel)
	}
	return nil
}

// -- FILE LOCAL -------------------------------------------------------------

func pointerPath(repoPath, rel string) string {
	return fsutil.PathOS(repoPath, ".vit", "workspace", "pointers", rel+".json")
}

func NewErrUntrackedFile(path string) error {
	return types.NewVitError(
		types.ErrPointerNotFound,
		[]string{fmt.Sprintf("unable to find tracked file at: %s", path)},
		[]any{"path", path},
	)
}
