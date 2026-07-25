package types

// CopyFilesRequest describe a copy operation to execute.
// DirStat.RootAbsPath : the SRC dir
// DirStat.Data : all the files to copy, path relative to SRC dir
// DstRoot abs path of root destination
type CopyFilesRequest struct {
	DstRoot string
	Src     *DirStat
}
