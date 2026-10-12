package types

import (
	"fmt"
	"strings"
	"time"
)

// Result types
// These structs define the JSON structure returned by each command
// and the strings returned by the cli (using the ToStringSlice() method)
// and are used to automatically generate Python TypedDict definitions
type Result interface {
	ToStringSlice() []string
}

// EmptyResult is returned by commands that don't produce output -------------
type EmptyResult struct{}

func (r *EmptyResult) ToStringSlice() []string {
	return []string{}
}

// StringResult return a simple string ---------------------------------------
type StringResult struct {
	String string `json:"string"`
}

func (r *StringResult) ToStringSlice() []string {
	return []string{r.String}
}

// StringListResult ----------------------------------------------------------
type StringListResult struct {
	StringList []string `json:"string_list"`
}

func (r *StringListResult) ToStringSlice() []string {
	return r.StringList
}

// HistoryLogResult is the project-wide activity log (newest first) ----------
type HistoryLogResult struct {
	Entries []HistoryEntry `json:"entries"`
}

func (r *HistoryLogResult) ToStringSlice() []string {
	out := make([]string, 0, len(r.Entries))
	for i := range r.Entries {
		e := r.Entries[i]
		line := fmt.Sprintf(
			"%s  %-12s %-12s %s",
			e.Timestamp.Local().Format("2006-01-02 15:04"),
			e.User,
			e.Command,
			e.AssetPath,
		)
		if e.Details != "" {
			line += "  " + e.Details
		}
		if e.Message != "" {
			line += fmt.Sprintf("  %q", e.Message)
		}
		out = append(out, line)
	}
	return out
}

// LsResult ------------------------------------------------------------------
type LsResult struct {
	Kind        string        `json:"kind"` // "dir" | "asset" | "object"
	Path        string        `json:"path"`
	Children    []LsChild     `json:"children,omitempty"`
	Branches    []string      `json:"branches,omitempty"`
	TagFamilies []LsTagFamily `json:"tag_families,omitempty"`
	Object      *LsObject     `json:"object,omitempty"`
}

type LsChild struct {
	Name string `json:"name"`
	Type string `json:"type"` // "dir" | "asset"
}

type LsTagFamily struct {
	Family string `json:"family"`
	Latest string `json:"latest"` // highest version name in the family (may be "")
}

type LsObject struct {
	Ref           *Ref      `json:"ref"`
	WorkspacePath string    `json:"workspace_path,omitempty"`
	Author        string    `json:"author"`
	Timestamp     time.Time `json:"timestamp"`
	Size          int64     `json:"size"`
	Sources       *DepGraph `json:"sources,omitempty"`
}

func (r *LsResult) ToStringSlice() []string {
	switch r.Kind {
	case "dir":
		out := make([]string, 0, len(r.Children))
		for _, c := range r.Children {
			if c.Type == "dir" {
				out = append(out, c.Name+"/")
			} else {
				out = append(out, c.Name)
			}
		}
		return out
	case "asset":
		out := make([]string, 0, len(r.Branches)+len(r.TagFamilies))
		for _, b := range r.Branches {
			out = append(out, "branch  "+b)
		}
		for _, f := range r.TagFamilies {
			line := "tag     " + f.Family
			if f.Latest != "" {
				line += "  (latest: " + f.Latest + ")"
			}
			out = append(out, line)
		}
		return out
	case "object":
		o := r.Object
		if o == nil {
			return nil
		}
		out := []string{o.Ref.SnapshotPath()}
		if o.WorkspacePath != "" {
			out = append(out, fmt.Sprintf("  path   : %s", o.WorkspacePath))
		}
		out = append(out,
			fmt.Sprintf("  author : %s", o.Author),
			fmt.Sprintf("  date   : %s", o.Timestamp.Local().Format("2006-01-02 15:04")),
			fmt.Sprintf("  size   : %s", humanSize(o.Size)),
		)
		if o.Sources != nil && len(o.Sources.Edges) > 0 {
			out = append(out, "  sources:")
			appendSourceTree(&out, o.Sources, o.Ref, 1)
		}
		return out
	}
	return nil
}

func appendSourceTree(out *[]string, g *DepGraph, cur *Ref, depth int) {
	if depth > 32 { // safety net (cycles are rejected at source-add)
		return
	}
	for i := range g.Edges {
		e := g.Edges[i]
		if e.Type != AssetDependencySource || !sameRefObject(e.Consumer, cur) {
			continue
		}
		spec := "?"
		if e.RefSpec != nil {
			spec = e.RefSpec.RelativePath()
		}
		// <name> :: <requested spec> -> <resolved>   (tabs for column alignment)
		*out = append(*out, fmt.Sprintf("%s%s\t:: %s\t-> %s",
			strings.Repeat("  ", depth+1), e.Name, spec, resolvedLabel(e.Provider)))
		appendSourceTree(out, g, e.Provider, depth+1)
	}
}

func resolvedLabel(r *Ref) string {
	switch r.RefType {
	case RefTypeCommit:
		return "commit=" + r.CommitID
	case RefTypeTag:
		if r.TagName != "" {
			return "name=" + r.TagName
		}
		return "tag=" + r.TagFamily
	case RefTypeBranch:
		return "branch=" + r.BranchName
	default:
		return r.ObjectPath()
	}
}

func sameRefObject(a, b *Ref) bool {
	return a.RepoUID == b.RepoUID && a.AssetPath == b.AssetPath && a.ObjectPath() == b.ObjectPath()
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// StatusResult --------------------------------------------------------------

type StatusResult ChangeRecord

func (c *StatusResult) HasChange() bool { return (*ChangeRecord)(c).HasChange() }

func (c *StatusResult) ToStringSlice() []string {
	if len(c.Files) == 0 {
		return []string{"Branch clean, no change to commit"}
	}

	var ret []string

	var added []string
	var edited []string
	var removed []string

	var sizeAdded int64
	var sizeRemoved int64
	for _, f := range c.Files {
		switch f.Type {
		case FileChangeAdded:
			added = append(added, f.Path)
			sizeAdded += f.Size
		case FileChangeRemoved:
			removed = append(removed, f.Path)
			sizeRemoved += f.Size
		case FileChangeEdited:
			edited = append(edited, f.Path)
			sizeAdded += f.Size
		default:
			panic("unreachable")
		}
	}

	ret = append(ret, "* added:")
	ret = append(ret, added...)
	ret = append(ret, "")
	ret = append(ret, "* removed:")
	ret = append(ret, removed...)
	ret = append(ret, "")
	ret = append(ret, "* edited:")
	ret = append(ret, edited...)
	ret = append(ret, "")
	ret = append(ret, fmt.Sprintf("+ %s  - %s", humanSize(sizeAdded), humanSize(sizeRemoved)))

	// TODO: formatter for sequence!

	return ret
}

// Source Update Result ------------------------------------------------------

type SourcePullResult struct {
	Consumer *Ref
	Updates  []SourceUpdate
}

func (r *SourcePullResult) ToStringSlice() []string {
	if len(r.Updates) == 0 {
		return []string{fmt.Sprintf("No source update done on: %s", r.Consumer.RelativePath())}
	}

	ret := []string{fmt.Sprintf("source updates on: %s", r.Consumer.RelativePath())}
	for _, u := range r.Updates {
		ret = append(ret, fmt.Sprintf(
			" - %s\t: %s -> %s",
			u.Name,
			u.OldTarget.RelativePath(),
			u.NewTarget.RelativePath(),
		))
	}
	return ret
}

type SourcePushResult struct {
	Producer  *Ref
	Consumers []SourcePullResult
}

func (r *SourcePushResult) ToStringSlice() []string {
	var moved int
	for _, c := range r.Consumers {
		if len(c.Updates) > 0 {
			moved++
		}
	}
	if moved == 0 {
		return []string{fmt.Sprintf("push %s: no consumer updated", r.Producer.RelativePath())}
	}
	ret := []string{fmt.Sprintf("push %s -> %d consumer(s) updated:", r.Producer.RelativePath(), moved)}
	for _, c := range r.Consumers {
		ret = append(ret, c.ToStringSlice()...)
	}
	return ret
}
