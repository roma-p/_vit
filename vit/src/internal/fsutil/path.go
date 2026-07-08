package fsutil

import (
	"path"
	"path/filepath"
)

// Vit supports windows and linux / mac meaning:
// - vit shall works normally on all those platform
// - a repo build on a posix system should work on windows and vice versa.
//
// So windows "\" paths and posix "/" path has to be handled.
//
// Internally, all paths are stored as posix path in the db.
// Only converted to Os path (either windows or posix) when needed.
//
// !! Don't use path.* or filepath.* directly if an helper is defined below!!
// (for code clarity)
//
// Usage rule: every public function with a path as argument shall normalize it
// (either to Internal or OS), for safety reason. A "normalize path" guard.
// private functions trust their callers.

// --- Join --- (can also be used to convert any path to OS / Internal.

func PathInternal(parts ...string) string {
	for i, p := range parts {
		parts[i] = filepath.ToSlash(p)
	}
	return path.Join(parts...)
}

func PathOS(parts ...string) string {
	for i, p := range parts {
		parts[i] = filepath.FromSlash(p)
	}
	return filepath.Join(parts...)
}

// --- Dir ---

func DirOS(p string) string { return filepath.Dir(filepath.FromSlash(p)) }

// --- Base ---

func BaseOS(p string) string { return filepath.Base(filepath.FromSlash(p)) }

func BaseInternal(p string) string { return path.Base(filepath.ToSlash(p)) }

// --- Ext ---

func ExtOS(p string) string { return filepath.Ext(filepath.FromSlash(p)) }

func ExtInternal(p string) string { return path.Ext(filepath.ToSlash(p)) }

// --- Abs ---

func AbsOS(p string) (string, error) { return filepath.Abs(filepath.FromSlash(p)) }

// --- Rel ---
func RelInternal(base, target string) (string, error) {
	rel, err := filepath.Rel(filepath.FromSlash(base), filepath.FromSlash(target))
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}
