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

	// Present links/creates one entry.
	Present(ctx context.Context, blobPath, workspacePath string, isDir, readOnly bool) error

	Alias(ctx context.Context, targetWorkspacePath, linkWorkspacePath string) error

	// Seal finalizes an immutable presentation: make every files / dir readOnly
	Seal(ctx context.Context, root string) error
}
