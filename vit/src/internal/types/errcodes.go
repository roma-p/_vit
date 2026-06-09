package types

const (
	ErrLockAcquireTimeout = "LockAcquireTimeout"

	ErrRepoInitFailed = "RepoInitFailed"
	ErrInvalidRef     = "InvalidRef"
	ErrFileNotFound   = "FileNotFound"

	ErrDBTreeNode = "DBTreeNode" // standard err when a tree node is missing, wrong type etc...
	ErrDBInternal = "DBInternal" // unexpected error at db level: a tree node pointing to a non
	// existing asset, a missing parent tree node etc...

	ErrAssetObjectNotFound      = "AssetObjectNotFound"
	ErrAssetObjectAlreadyExists = "AssetObjectAlreadyExists"
)
