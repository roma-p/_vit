package testutils

import (
	"os"
	"strings"
	"testing"
)

func AssertExists(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("Path %s does not exist", path)
	}
}

func AssertNotExists(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); err == nil {
		t.Fatalf("Path %s should not exist", path)
	}
}

func AssertEqual[T comparable](t *testing.T, got, expected T) {
	t.Helper()

	if got != expected {
		t.Fatalf("got %v, expected %v", got, expected)
	}
}

func AssertNotEqual[T comparable](t *testing.T, got, expected T) {
	t.Helper()

	if got != expected {
		t.Fatalf("got %v, unexpected equalt to %v", got, expected)
	}
}

func AssertSliceEqual[T comparable](t *testing.T, got, expected []T) {
	t.Helper()

	// TODO: better display of err?

	if len(got) != len(expected) {
		t.Fatalf("got %v, expected %v", got, expected)
	} else {
		for i, v := range got {
			if v != expected[i] {
				t.Fatalf("got %v, expected %v", got, expected)
			}
		}
	}
}

func AssertNoError(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func AssertError(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

func AssertErrorContains(t *testing.T, err error, substr string) {
	t.Helper()

	if err == nil {
		t.Fatal("expected error but got nil")
	}

	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("expected error to contain %q, but got: %v", substr, err)
	}
}

func AssertTrue(t *testing.T, condition bool) {
	t.Helper()

	if !condition {
		t.Fatal("expected condition to be true")
	}
}

func AssertFalse(t *testing.T, condition bool) {
	t.Helper()

	if condition {
		t.Fatal("expected condition to be false")
	}
}
