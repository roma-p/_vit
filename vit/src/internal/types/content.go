package types

import (
	"fmt"
	"path"
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
	var b strings.Builder
	for _, e := range n.Children {
		fmt.Fprintf(&b, "d %s %s\n", e.ContentHash, e.Name)
	}
	for _, e := range n.Files {
		fmt.Fprintf(&b, "f %s %d %s\n", FormatTimestamp(e.MTime), e.Size, e.BlobTarget)
	}
	return []byte(b.String())
}

func ParseContentNode(b []byte) (*ContentNode, error) {
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
