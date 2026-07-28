package workspace

import (
	"context"
	"fmt"
	"os"

	"vit/internal/fsutil"
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
	pool *fsutil.JSONHandlerPool,
	repoPath, path string,
) (*types.Ref, error) {
	if ref, err := types.NewRefFromPath(repoPath, path); err == nil {
		return ref, nil
	}

	// path is already repo-relative (WithOperationContext normalized it, same as
	// NewRefFromPath expects) — walk it up to the nearest pointer.
	for cur := fsutil.PathInternal(path); cur != "." && cur != "/" && cur != ""; cur = fsutil.DirInternal(cur) {
		pp := pointerPath(repoPath, cur)
		if _, err := os.Stat(pp); err != nil {
			continue
		}
		h, err := fsutil.ResolveHandler[Pointer](ctx, pool, pp, false, nil)
		if err != nil {
			return nil, NewErrUntrackedFile(path)
		}
		ref := h.Data.Ref
		return &ref, nil
	}
	return nil, NewErrUntrackedFile(path)
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
