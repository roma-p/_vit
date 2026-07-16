package types

import "fmt"

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

// StatusResult --------------------------------------------------------------

type StatusResult struct {
	Change           bool
	FileChange       map[string]StatusResultFileChange
	DependancyChange []any // TODO: change this
}

type StatusResultFileChange struct {
	Type ChangeType
	Path string
}

func (c *StatusResult) ToStringSlice() []string {
	var ret []string

	var added []string
	var edited []string
	var removed []string

	for f, fileChange := range c.FileChange {
		switch fileChange.Type {
		case Added:
			added = append(added, f)
		case Removed:
			removed = append(removed, f)
		case Edited:
			edited = append(edited, f)
		default:
			panic("unreachable")
		}
	}

	for _, f := range added {
		ret = append(ret, fmt.Sprintf("added   : %s", f))
	}
	for _, f := range removed {
		ret = append(ret, fmt.Sprintf("removed : %s", f))
	}
	for _, f := range edited {
		ret = append(ret, fmt.Sprintf("edited  : %s", f))
	}
	return ret
}
