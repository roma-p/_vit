package fsutil

import (
	"os"
	"path/filepath"
	"testing"

	testutils "vit/internal/testhelpers"
)

type testData struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestJSONHandler_WriteAndRead(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "json-test-*")
	defer cleanup()

	path := filepath.Join(tmpDir, "data.json")
	handler := NewJSONHandlerFromPath(path, &testData{Name: "hello", Value: 42})

	err := handler.Write()
	testutils.AssertNoError(t, err)
	testutils.AssertExists(t, path)

	// Read back
	loaded, err := NewJSONHandlerFromFile[testData](path)
	testutils.AssertNoError(t, err)
	testutils.AssertEqual(t, loaded.Data.Name, "hello")
	testutils.AssertEqual(t, loaded.Data.Value, 42)
}

func TestJSONHandler_Read(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "json-test-*")
	defer cleanup()

	path := filepath.Join(tmpDir, "data.json")
	handler := NewJSONHandlerFromPath(path, &testData{Name: "initial", Value: 1})
	testutils.AssertNoError(t, handler.Write())

	// Modify on disk
	handler2 := NewJSONHandlerFromPath(path, &testData{Name: "updated", Value: 2})
	testutils.AssertNoError(t, handler2.Write())

	// Read should pick up disk changes
	testutils.AssertNoError(t, handler.Read())
	testutils.AssertEqual(t, handler.Data.Name, "updated")
	testutils.AssertEqual(t, handler.Data.Value, 2)
}

func TestJSONHandler_CreatesParentDirs(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "json-test-*")
	defer cleanup()

	path := filepath.Join(tmpDir, "deep", "nested", "data.json")
	handler := NewJSONHandlerFromPath(path, &testData{Name: "nested", Value: 99})

	err := handler.Write()
	testutils.AssertNoError(t, err)
	testutils.AssertExists(t, path)
}

func TestJSONHandler_ReadNotFound(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "json-test-*")
	defer cleanup()

	path := filepath.Join(tmpDir, "nonexistent.json")
	_, err := NewJSONHandlerFromFile[testData](path)
	testutils.AssertError(t, err)
}

func TestJSONHandler_ReadInvalidJSON(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "json-test-*")
	defer cleanup()

	path := filepath.Join(tmpDir, "bad.json")
	os.WriteFile(path, []byte("{not valid json}"), 0o644)

	_, err := NewJSONHandlerFromFile[testData](path)
	testutils.AssertError(t, err)
}

func TestJSONHandler_ReadRejectsUnknownFields(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "json-test-*")
	defer cleanup()

	path := filepath.Join(tmpDir, "extra.json")
	os.WriteFile(path, []byte(`{"name":"ok","value":1,"unknown":"field"}`), 0o644)

	_, err := NewJSONHandlerFromFile[testData](path)
	testutils.AssertError(t, err)
}

func TestJSONHandler_FilePath(t *testing.T) {
	handler := NewJSONHandlerFromPath("/some/path.json", &testData{})
	testutils.AssertEqual(t, handler.FilePath(), "/some/path.json")
}

func TestJSONHandler_AnyJSONHandlerInterface(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "json-test-*")
	defer cleanup()

	path := filepath.Join(tmpDir, "iface.json")
	handler := NewJSONHandlerFromPath(path, &testData{Name: "iface", Value: 7})

	// Should satisfy AnyJSONHandler
	var any AnyJSONHandler = handler
	testutils.AssertNoError(t, any.Write())
	testutils.AssertEqual(t, any.FilePath(), path)
	testutils.AssertNoError(t, any.Read())
}

func TestJSONHandler_Overwrite(t *testing.T) {
	tmpDir, cleanup := testutils.TempDir(t, "json-test-*")
	defer cleanup()

	path := filepath.Join(tmpDir, "data.json")

	h1 := NewJSONHandlerFromPath(path, &testData{Name: "first", Value: 1})
	testutils.AssertNoError(t, h1.Write())

	h2 := NewJSONHandlerFromPath(path, &testData{Name: "second", Value: 2})
	testutils.AssertNoError(t, h2.Write())

	loaded, err := NewJSONHandlerFromFile[testData](path)
	testutils.AssertNoError(t, err)
	testutils.AssertEqual(t, loaded.Data.Name, "second")
	testutils.AssertEqual(t, loaded.Data.Value, 2)
}
