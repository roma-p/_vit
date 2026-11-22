package cli

import (
	"maps"
	"testing"
)

func TestParseAttrValues(t *testing.T) {
	got, err := parseAttrValues([]string{"task=mod", "omit=", "expr=a=b"})
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"task": "mod", "omit": "", "expr": "a=b"}; !maps.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	for _, args := range [][]string{{"task"}, {"task=a", "task=b"}} {
		if _, err := parseAttrValues(args); err == nil {
			t.Errorf("%v: want an error", args)
		}
	}
}
