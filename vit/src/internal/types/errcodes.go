package types

// TODO: le derbol, a reorganiser à un moment

const (
	ErrInternalRepoCorrupted = "RepoCorrupted"

	ErrLockAcquireTimeout = "LockAcquireTimeout"

	ErrRepoInitFailed = "RepoInitFailed"
	ErrInvalidRef     = "InvalidRef"
	ErrFileNotFound   = "FileNotFound"

	ErrDBTreeNode = "DBTreeNode" // standard err when a tree node is missing, wrong type etc...
	ErrDBInternal = "DBInternal" // unexpected error at db level: a tree node pointing to a non
	// existing asset, a missing parent tree node etc...

	ErrBlobInternal = "BlobInternal" // unexpected error at blob level: failed copy, failed
	// content-node write etc...

	ErrWorkspaceInternal = "WorkspaceInternal" // unexpected error presenting to the workspace:
	// failed hardlink/mkdir, hardlinks unsupported (cross-device, Windows no dev mode) etc...

	ErrHistoryInternal = "HistoryInternal" // unexpected error reading the activity log

	ErrPointerNotFound = "PointerNotFound" // no ref/pointer resolves the given workspace path

	ErrAssetObjectNotFound      = "AssetObjectNotFound"
	ErrAssetObjectAlreadyExists = "AssetObjectAlreadyExists"

	ErrDependencyNotFound      = "DependencyNotFound"
	ErrDependencyAlreadyExists = "DependencyAlreadyExists"
	ErrCircularDependency      = "CircularDependency"

	ErrTransactionConflict = "TransactionConflict"
	ErrTransactionFailed   = "TransactionFailed"

	ErrInvalidInut = "InvalidInput"

	ErrNoChangeToCommit = "NoChangeToCommit"

	ErrBranchDirty = "BranchDirty" // branching from a branch with uncommitted changes
)
