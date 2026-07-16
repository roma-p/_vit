package types

import (
	"time"
)

type FileStat struct {
	Size  int64
	MTime time.Time
}

type DirStat struct {
	Data        map[string]FileStat //  keys are relative path. Use for diffing.
	RootAbsPath string
}

func NewDirStat(rootAbsPath string) *DirStat {
	return &DirStat{
		Data:        make(map[string]FileStat),
		RootAbsPath: rootAbsPath,
	}
}

func (a *DirStat) Diff(b *DirStat, baselineTime time.Time) *ChangeRecord {
	var ret ChangeRecord
	for path, aStat := range a.Data {
		bStat, ok := b.Data[path]
		if !ok {
			ret.AddChange(path, FileChangeAdded, aStat.Size)
		} else if aStat.MTime.After(baselineTime) || aStat.Size != bStat.Size {
			ret.AddChange(path, FileChangeEdited, aStat.Size)
		}
	}

	for path := range b.Data {
		if _, ok := a.Data[path]; !ok {
			ret.AddChange(path, FileChangeRemoved, 0)
		}
	}

	// TODO: fill ChangeRecord.Dependancies when dependancy management done!

	return &ret
}
