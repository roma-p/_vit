package types

import (
	"fmt"
	// Exception to the "no raw path.*/filepath.* outside fsutil/path.go" convention:
	// okay because we know it will always be poxix here (internal)
	// but myabe serialize unserialize to move?
	// OR path utils to move to tyes?
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ContentNode struct {
	// Payload
	Name     string
	Children []*ContentNode
	Files    []*ContentNodeFile

	// Derived from payload
	Serialized  []byte
	ContentHash string
}

type ContentNodeFile struct {
	Name       string
	Size       int64
	MTime      time.Time
	BlobTarget string
}

func (n *ContentNode) Serialize() []byte {
	children := append([]*ContentNode(nil), n.Children...)
	files := append([]*ContentNodeFile(nil), n.Files...)
	sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
	sort.Slice(files, func(i, j int) bool { return files[i].BlobTarget < files[j].BlobTarget })

	var b strings.Builder
	for _, e := range children {
		fmt.Fprintf(&b, "d %s %s\n", e.ContentHash, e.Name)
	}
	for _, e := range files {
		fmt.Fprintf(&b, "f %s %d %s\n", FormatTimestamp(e.MTime), e.Size, e.BlobTarget)
	}
	return []byte(b.String())
}

func DeserializeContentNode(b []byte) (*ContentNode, error) {
	n := &ContentNode{Serialized: b}
	for i, line := range strings.Split(string(b), "\n") {
		if line == "" {
			continue
		}
		if len(line) < 2 || line[1] != ' ' {
			return nil, fmt.Errorf("content: malformed node line %d: %q", i+1, line)
		}
		switch line[0] {
		case 'd': // d <child-content-hash> <name>
			parts := strings.SplitN(line, " ", 3)
			if len(parts) != 3 {
				return nil, fmt.Errorf("content: malformed dir line %d: %q", i+1, line)
			}
			n.Children = append(n.Children, &ContentNode{
				ContentHash: parts[1],
				Name:        parts[2],
			})
		case 'f': // f <mtime> <size> <blobtarget>
			parts := strings.SplitN(line, " ", 4)
			if len(parts) != 4 {
				return nil, fmt.Errorf("content: malformed file line %d: %q", i+1, line)
			}
			mtime, err := ParseTimestamp(parts[1])
			if err != nil {
				return nil, fmt.Errorf("content: bad mtime on line %d: %w", i+1, err)
			}
			size, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("content: bad size on line %d: %w", i+1, err)
			}
			blobTarget := parts[3]
			n.Files = append(n.Files, &ContentNodeFile{
				Name:       path.Base(blobTarget),
				Size:       size,
				MTime:      mtime,
				BlobTarget: blobTarget,
			})
		default:
			return nil, fmt.Errorf("content: unknown entry kind %q on line %d", line[0], i+1)
		}
	}
	return n, nil
}

func (n *ContentNode) ToDirStat(fullPath string) *DirStat {
	ret := NewDirStat(fullPath)
	n.collectDirStat("", ret)
	return ret
}

func (n *ContentNode) collectDirStat(prefix string, dirStat *DirStat) {
	for _, f := range n.Files {
		dirStat.Data[path.Join(prefix, f.Name)] = FileStat{Size: f.Size, MTime: f.MTime}
	}
	for _, c := range n.Children {
		c.collectDirStat(path.Join(prefix, c.Name), dirStat)
	}
}
