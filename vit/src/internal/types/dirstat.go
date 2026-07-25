package types

import (
	"maps"
	"sort"
	"time"
)

type DirStat struct {
	Data        map[FilePath]FileStat // path relative to RootAbsPath
	RootAbsPath string
}

type FilePath = string

type FileStat struct {
	Size  int64
	MTime time.Time
}

func NewDirStat(rootAbsPath string) *DirStat {
	return &DirStat{
		Data:        make(map[string]FileStat),
		RootAbsPath: rootAbsPath,
	}
}

// WithRoot returns a deepcopy with the changed root.
func (d *DirStat) WithRoot(root string) *DirStat {
	return &DirStat{Data: maps.Clone(d.Data), RootAbsPath: root}
}

// Diff builds a ChangeRecord from two DirStat object. Works if the two RootAbsPath are different.
func (d *DirStat) Diff(other *DirStat, baselineTime time.Time) *ChangeRecord {
	var ret ChangeRecord
	for path, aStat := range d.Data {
		bStat, ok := other.Data[path]
		if !ok {
			ret.AddChange(path, FileChangeAdded, aStat.Size)
		} else if aStat.MTime.After(baselineTime) || aStat.Size != bStat.Size {
			ret.AddChange(path, FileChangeEdited, aStat.Size)
		}
	}

	for path := range other.Data {
		if _, ok := d.Data[path]; !ok {
			ret.AddChange(path, FileChangeRemoved, 0)
		}
	}

	sort.Slice(ret.Files, func(i, j int) bool {
		return ret.Files[i].Path < ret.Files[j].Path
	})

	return &ret
}
