package types

// CopyFilesRequest describe a copy operation to execute.
// for each DirStat in Src:
// DirStat.RootAbsPath : the SRC dir
// DirStat.Data : all the files to copy, path relative to SRC dir
// DstRoot abs path of root destination
// Writable: make either readonly or read write the dst files.
type CopyFilesRequest struct {
	DstRoot  string
	Src      []*DirStat
	Writable bool
}
