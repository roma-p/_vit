package types

const (
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

	ErrAssetObjectNotFound      = "AssetObjectNotFound"
	ErrAssetObjectAlreadyExists = "AssetObjectAlreadyExists"

	ErrTransactionConflict = "TransactionConflict"
	ErrTransactionFailed   = "TransactionFailed"
)
