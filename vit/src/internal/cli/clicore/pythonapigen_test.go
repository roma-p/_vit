package clicore

import "testing"

func TestPythonIdentEscapesKeywords(t *testing.T) {
	for name, want := range map[string]string{"with": "with_", "from": "from_", "repo-uid": "repo_uid", "without": "without", "set": "set"} {
		if got := pythonIdent(name); got != want {
			t.Errorf("%s: got %s, want %s", name, got, want)
		}
	}
}
