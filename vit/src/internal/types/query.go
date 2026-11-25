package types

import (
	"maps"
	"slices"
	"strings"
)

type QueryRequest struct {
	With     []string          `json:"with,omitempty"`
	Without  []string          `json:"without,omitempty"`
	Eq       map[string]string `json:"eq,omitempty"`
	Fetch    []string          `json:"fetch,omitempty"`
	Asset    bool              `json:"asset,omitempty"`
	Distinct bool              `json:"distinct,omitempty"`
	Set      bool              `json:"set,omitempty"`
}

type QueryResult struct {
	Rows []QueryRow          `json:"rows,omitempty"`
	Sets map[string][]string `json:"sets,omitempty"`

	fetch []string
}

type QueryRow struct {
	Asset string            `json:"asset,omitempty"`
	Attrs map[string]string `json:"attrs,omitempty"`
}

func (r *QueryResult) SetFetchOrder(keys []string) {
	r.fetch = keys
}

func (r *QueryResult) ToStringSlice() []string {
	if r.Sets != nil {
		out := make([]string, 0, len(r.Sets))
		for _, k := range fetchOrder(r.fetch, r.Sets) {
			out = append(out, k+": "+strings.Join(r.Sets[k], ", "))
		}
		return out
	}
	out := make([]string, 0, len(r.Rows))
	for _, row := range r.Rows {
		var parts []string
		if row.Asset != "" {
			parts = append(parts, row.Asset)
		}
		for _, k := range fetchOrder(r.fetch, row.Attrs) {
			parts = append(parts, k+"="+row.Attrs[k])
		}
		out = append(out, strings.Join(parts, "  "))
	}
	return out
}

func fetchOrder[V any](fetch []string, present map[string]V) []string {
	if fetch != nil {
		return fetch
	}
	return slices.Sorted(maps.Keys(present))
}
