// Package shred provides file-overwrite and removal functionality.
package shred

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

var (
	// ErrMultipleNamesNotAllowed is returned when the file that shred
	// receives has more than one name.
	ErrMultipleNamesNotAllowed = errors.New("multiple names not allowed")

	// ErrPathChanged is returned when the file path is changed after
	// being inspected by Lstat but before being opened.
	ErrPathChanged = errors.New("path identity changed")
)

// testHookAfterLstat is used only by tests to force a filesystem change
// between Lstat and OpenFile. In normal operation it is nil.
var testHookAfterLstat func()

// passes is the number of times the file content is overwritten.
const passes = 3

func overwrite(file *os.File, size int64) error {
	for pass := 1; pass <= passes; pass++ {

		// Move the file offset to the first byte.
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("seek before pass %d: %w", pass, err)
		}

		// Overwrite the complete file with random data.
		if _, err := io.CopyN(file, rand.Reader, size); err != nil {
			return fmt.Errorf("overwrite pass %d: %w", pass, err)
		}

		// Flush file data from kernel buffers to storage.
		if err := file.Sync(); err != nil {
			return fmt.Errorf("sync after pass %d: %w", pass, err)
		}
	}

	return nil
}

// shred overwrites the regular file at path passes times with
// cryptographically secure random data, syncs each pass to storage,
// closes the file and finally removes it. Multiply-linked files are
// refused.
func shred(path string) error {

	// Inspect the path itself, without following a symbolic link and
	// without opening it.
	linfo, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("shred: lstat %q: %w", path, err)
	}

	// Verify if file is a regular file.
	if !linfo.Mode().IsRegular() {
		return fmt.Errorf(
			"shred: %q is not a regular file: %w",
			path,
			fs.ErrInvalid,
		)
	}

	// Test hook: allows a test to change the filesystem after Lstat
	// but before OpenFile.
	if testHookAfterLstat != nil {
		testHookAfterLstat()
	}

	// Open file.
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("shred: open %q: %w", path, err)
	}

	// Ensure file will be closed before execution of shred exits.
	// This way file will be closed in any way execution ends, be it
	// because the end of the code was reached or because an early
	// return command was issued.
	defer func() {
		if file != nil {
			_ = file.Close()
		}
	}()

	// Get file metadata (Name, Size, Mode, ModTime and IsDir), needed
	// to run the SameFile check below.
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("shred: stat %q: %w", path, err)
	}

	// Guard against the path being swapped (for example for a symbolic
	// link) between the Lstat and the open.
	if !os.SameFile(linfo, info) {
		return fmt.Errorf(
			"shred: file path %q changed before opening: %w",
			path,
			ErrPathChanged,
		)
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf(
			"shred: the link count could not be read for %q: %w",
			path,
			errors.ErrUnsupported,
		)
	}

	// Multiply-linked files (Nlink > 1) are refused.
	if stat.Nlink > 1 {
		return fmt.Errorf(
			"shred: file %q was refused and left "+
				"untouched because it has %d names: %w",
			path,
			stat.Nlink,
			ErrMultipleNamesNotAllowed,
		)
	}

	// Overwrite the complete file passes times.
	size := info.Size()

	if err := overwrite(file, size); err != nil {
		return fmt.Errorf("shred: %w", err)
	}

	// Close the file.
	err = file.Close()
	file = nil

	if err != nil {
		return fmt.Errorf("shred: close %q: %w", path, err)
	}

	// Delete the file name from the file system.
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("shred: remove %q: %w", path, err)
	}

	// Sync to persist deletion.
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}

	return nil
}
