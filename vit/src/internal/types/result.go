
package types

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
