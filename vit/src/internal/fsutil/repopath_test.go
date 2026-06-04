package fsutil

import (
	"path/filepath"
	"testing"
	testutils "vit/internal/testhelpers"
)

func TestCheckPathIsVitRepoOk(t *testing.T) {
	tempDir, cleanup := testutils.TempDir(t, "test_repo_path_vit_repo_*")
	defer cleanup()
	testutils.CreateDirectories(t, tempDir, []string{"path/to/ok/.vit"})

	s := CheckPathIsVitRepo(filepath.Join(tempDir, "path/to/ok"))
	testutils.AssertEqual(t, s, true)
}

func TestCheckPathIsVitRepoNonExistent(t *testing.T) {
	tempDir, cleanup := testutils.TempDir(t, "test_repo_path_non_existent_*")
	defer cleanup()

	s := CheckPathIsVitRepo(filepath.Join(tempDir, "does/not/exist"))
	testutils.AssertEqual(t, s, false)
}

func TestCheckPathIsVitRepoFileNotDir(t *testing.T) {
	tempDir, cleanup := testutils.TempDir(t, "test_repo_path_file_*")
	defer cleanup()
	testutils.CreateDirectories(t, tempDir, []string{"path/to/repo"})

	vitFilePath := filepath.Join(tempDir, "path/to/repo/.vit")
	testutils.CreateFile(t, vitFilePath, "not a directory")

	s := CheckPathIsVitRepo(filepath.Join(tempDir, "path/to/repo"))
	testutils.AssertEqual(t, s, false)
}

func TestFindVitRepoFromPathDirectRepo(t *testing.T) {
	tempDir, cleanup := testutils.TempDir(t, "test_find_repo_direct_*")
	defer cleanup()
	testutils.CreateDirectories(t, tempDir, []string{"repo/.vit"})

	repoPath, relativePath, err := FindVitRepoFromPath(filepath.Join(tempDir, "repo"), false)

	testutils.AssertNoError(t, err)
	testutils.AssertEqual(t, repoPath, filepath.Join(tempDir, "repo"))
	testutils.AssertEqual(t, relativePath, "")
}

func TestFindVitRepoFromPathNestedPath(t *testing.T) {
	tempDir, cleanup := testutils.TempDir(t, "test_find_repo_nested_*")
	defer cleanup()
	testutils.CreateDirectories(t, tempDir, []string{"repo/.vit", "repo/subdir/deep/path"})

	repoPath, relativePath, err := FindVitRepoFromPath(
		filepath.Join(tempDir, "repo/subdir/deep/path"),
		false,
	)

	testutils.AssertNoError(t, err)
	testutils.AssertEqual(t, repoPath, filepath.Join(tempDir, "repo"))
	testutils.AssertEqual(t, relativePath, "subdir/deep/path")
}

func TestFindVitRepoFromPathNotFound(t *testing.T) {
	tempDir, cleanup := testutils.TempDir(t, "test_find_repo_not_found_*")
	defer cleanup()
	testutils.CreateDirectories(t, tempDir, []string{"not/a/repo"})

	_, _, err := FindVitRepoFromPath(filepath.Join(tempDir, "not/a/repo"), false)

	testutils.AssertError(t, err)
}

func TestFindVitRepoFromPathNonExistentIgnore(t *testing.T) {
	tempDir, cleanup := testutils.TempDir(t, "test_find_repo_ignore_*")
	defer cleanup()
	testutils.CreateDirectories(t, tempDir, []string{"repo/.vit"})

	repoPath, relativePath, err := FindVitRepoFromPath(
		filepath.Join(tempDir, "repo/does/not/exist"),
		true,
	)

	testutils.AssertNoError(t, err)
	testutils.AssertEqual(t, repoPath, filepath.Join(tempDir, "repo"))
	testutils.AssertEqual(t, relativePath, "does/not/exist")
}

func TestFindVitRepoFromPathNonExistentNoIgnore(t *testing.T) {
	tempDir, cleanup := testutils.TempDir(t, "test_find_repo_no_ignore_*")
	defer cleanup()

	_, _, err := FindVitRepoFromPath(
		filepath.Join(tempDir, "does/not/exist"),
		false,
	)

	testutils.AssertError(t, err)
}
