package fsutil

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// JSONHandler is a wrapper around JSON operation
// but Writing / Reading concurrency not handled! (see fsutil.lockfile.go for this)
type JSONHandler[T any] struct {
	Filepath string
	Data     *T
}

// FilePath returns the file path of the handler.
func (j *JSONHandler[T]) FilePath() string {
	return j.Filepath
}

// AnyJSONHandler is the type-erased interface for JSONHandler[T].
// Use it when you need to hold handlers of different T in the same data structure
// that is the case for: JSONHandlerPool
// !!! Typed access to .Data requires asserting back to the correct *JSONHandler[T].
type AnyJSONHandler interface {
	FilePath() string
	Read() error
	Write() error
}

// NewJSONHandlerFromFile loads a JSON file from disk into a new handler.
func NewJSONHandlerFromFile[T any](filepath string) (*JSONHandler[T], error) {
	data, err := jsonRead[T](filepath)
	if err != nil {
		return nil, err
	}
	return &JSONHandler[T]{
		Filepath: filepath,
		Data:     data,
	}, nil
}

// NewJSONHandlerFromPath creates a new handler for a given path and sets its internal data.
// will not read disk.
func NewJSONHandlerFromPath[T any](filepath string, data *T) *JSONHandler[T] {
	return &JSONHandler[T]{
		Filepath: filepath,
		Data:     data,
	}
}

// Write writes the handler's data to disk atomically (temp file + rename)
// so concurrent readers never see partial data.
// Caller is responsible for holding the write lock (via JSONHandlerPool).
// Creates parent directories if they don't exist.
func (j *JSONHandler[T]) Write() error {
	return jsonWrite(j.Filepath, j.Data)
}

// Read loads the JSON file from disk into the handler's Data field.
func (j *JSONHandler[T]) Read() error {
	data, err := jsonRead[T](j.Filepath)
	if err != nil {
		return err
	}
	j.Data = data
	return nil
}

const jsonReadMaxRetries = 3
const jsonReadRetryDelay = 100 * time.Millisecond

func jsonRead[T any](filename string) (*T, error) {
	var lastErr error
	for attempt := range jsonReadMaxRetries {
		data, err := jsonReadOnce[T](filename)
		if err == nil {
			return data, nil
		}
		// Only retry on decode errors (likely a concurrent write mid-rename).
		// File-not-found or permission errors won't resolve with a retry.
		if !isDecodeError(err) {
			return nil, err
		}
		lastErr = err
		if attempt < jsonReadMaxRetries-1 {
			time.Sleep(jsonReadRetryDelay)
		}
	}
	return nil, fmt.Errorf("failed to read JSON after %d attempts: %w", jsonReadMaxRetries, lastErr)
}

func jsonReadOnce[T any](filename string) (*T, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()

	var data T
	if err := decoder.Decode(&data); err != nil {
		return nil, &jsonDecodeError{filename: filename, err: err}
	}
	return &data, nil
}

type jsonDecodeError struct {
	filename string
	err      error
}

func (e *jsonDecodeError) Error() string {
	return fmt.Sprintf("failed to decode JSON %s: %s", e.filename, e.err)
}

func (e *jsonDecodeError) Unwrap() error {
	return e.err
}

func isDecodeError(err error) bool {
	_, ok := err.(*jsonDecodeError)
	return ok
}

func jsonWrite[T any](filename string, data *T) error {
	return WriteFileAtomic(filename, 0o644, func(f *os.File) error {
		encoder := json.NewEncoder(f)
		encoder.SetIndent("", "  ")
		encoder.SetEscapeHTML(false)
		return encoder.Encode(data)
	})
}
