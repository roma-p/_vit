package types

import (
	"slices"
	"testing"
)

func TestQueryResultToStringSlice(t *testing.T) {
	row := func(asset string, attrs map[string]string) QueryRow { return QueryRow{Asset: asset, Attrs: attrs} }
	cases := []struct {
		name  string
		res   QueryResult
		fetch []string
		want  []string
	}{
		{"asset and fetch", QueryResult{Rows: []QueryRow{row("s20/sh010", map[string]string{"task": "mod", "shot": "010"})}},
			[]string{"task", "shot"}, []string{"s20/sh010  task=mod  shot=010"}},
		{"fetch only", QueryResult{Rows: []QueryRow{row("", map[string]string{"shot": "010"}), row("", map[string]string{"shot": "020"})}},
			[]string{"shot"}, []string{"shot=010", "shot=020"}},
		{"asset only", QueryResult{Rows: []QueryRow{row("s20/sh010", nil), row("s20/sh020", nil)}},
			nil, []string{"s20/sh010", "s20/sh020"}},
		{"set", QueryResult{Sets: map[string][]string{"shot": {"010", "020"}, "task": {"lgt", "mod"}}},
			[]string{"task", "shot"}, []string{"task: lgt, mod", "shot: 010, 020"}},
		{"no fetch order", QueryResult{Rows: []QueryRow{row("", map[string]string{"task": "mod", "shot": "010"})}},
			nil, []string{"shot=010  task=mod"}},
	}
	for _, c := range cases {
		c.res.SetFetchOrder(c.fetch)
		if got := c.res.ToStringSlice(); !slices.Equal(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
