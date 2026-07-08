package types

import (
	"fmt"
	"strings"
	"time"
)

type ContentNode struct {
	Name     string
	Children []*ContentNode
	Files    []*ContentNodeFile
	UID      string
}

type ContentNodeFile struct {
	Name       string
	Size       int64
	MTime      time.Time
	BlobTarget string
}

func (n *ContentNode) Serialize() []byte {
	var b strings.Builder
	for _, e := range n.Children {
		fmt.Fprintf(&b, "d %s %s\n", e.UID, e.Name)
	}
	for _, e := range n.Files {
		fmt.Fprintf(&b, "f %s %d %s\n", FormatTimestamp(e.MTime), e.Size, e.BlobTarget)
	}
	return []byte(b.String())
}
