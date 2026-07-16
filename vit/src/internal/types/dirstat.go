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

// Diff reports how a (the working tree) differs from b (the baseline commit).
// baselineTime is the branch's clean-state mtime floor: a file counts as Edited
// when its mtime is past the floor OR its size no longer matches the baseline
// (size catches in-place, mtime-preserving edits).
func (a *DirStat) Diff(b *DirStat, baselineTime time.Time) Changes {
	var ret Changes
	for path, aStat := range a.Data {
		bStat, ok := b.Data[path]
		if !ok {
			ret.AddChange(path, Added, aStat.Size)
		} else if aStat.MTime.After(baselineTime) || aStat.Size != bStat.Size {
			ret.AddChange(path, Edited, aStat.Size)
		}
	}

	for path := range b.Data {
		if _, ok := a.Data[path]; !ok {
			ret.AddChange(path, Removed, 0)
		}
	}

	return ret
}
