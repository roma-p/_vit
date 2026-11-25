package cli

import (
	"maps"
	"slices"
	"testing"
)

func TestQueryRequestFromArgs(t *testing.T) {
	p := buildQueryArgParser()
	if err := p.Parse([]string{"s20", "--eq", "seq=20,task=mod", "--with", "shot", "--without", "dcc,lgt", "--fetch", "shot,task", "--distinct"}); err != nil {
		t.Fatal(err)
	}
	req, err := queryRequestFromArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	if p.GetArg("path") != "s20" || !maps.Equal(req.Eq, map[string]string{"seq": "20", "task": "mod"}) ||
		!slices.Equal(req.With, []string{"shot"}) || !slices.Equal(req.Without, []string{"dcc", "lgt"}) ||
		!slices.Equal(req.Fetch, []string{"shot", "task"}) || !req.Distinct || req.Asset || req.Set {
		t.Fatalf("request = %+v", req)
	}

	for _, eq := range []string{"seq", "seq=20,seq=30"} {
		p := buildQueryArgParser()
		if err := p.Parse([]string{"--eq", eq, "--asset"}); err != nil {
			t.Fatal(err)
		}
		if _, err := queryRequestFromArgs(p); err == nil {
			t.Errorf("--eq %s: want an error", eq)
		}
	}
}
