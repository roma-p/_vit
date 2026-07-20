package types

type CopyFilesRequest struct {
	DstRoot string
	Src     *DirStat
}
