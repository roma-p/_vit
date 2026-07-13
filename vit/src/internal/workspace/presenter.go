package workspace

import (
	"context"
)

// Presenter is responsible for presenting files from blob to workspace
//
// Implementations can use different strategies (probably: hardlinks or copies)
type Presenter interface {
	// Check verifies if the presenter can operate on the current platform.
	// Returns error if the presenter's strategy is not supported.
	Check() error

	// Unpresent removes a file from the workspace.
	Unpresent(workspacePath string) error

	// Present links/creates one entry. Directories are made writable so files can
	// be linked in; PresentFinish seals them afterwards.
	Present(ctx context.Context, blobPath, workspacePath string, isDir, readOnly bool) error

	// PresentFinish finalizes a directory once all its files and subdirs are
	// presented (post-order). Seals it read-only for immutable presentations.
	PresentFinish(ctx context.Context, workspacePath string, readOnly bool) error
}
