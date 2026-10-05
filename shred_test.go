package shred

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// assertRemoved fails the test if path still exists.
func assertRemoved(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%q still exists after shred (err = %v)", path, err)
	}
}

func TestShredEmptyPath(t *testing.T) {
	if err := shred(""); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestShredNonexistentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist")

	err := shred(path)

	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}

	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got: %v", err)
	}
}

func TestShredDirectory(t *testing.T) {
	dir := t.TempDir()

	err := shred(dir)

	if err == nil {
		t.Fatal("expected error for directory")
	}

	if !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("expected fs.ErrInvalid, got: %v", err)
	}

	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("directory should be untouched: %v", err)
	}
}

func TestShredEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.txt")

	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}

	if err := shred(path); err != nil {
		t.Fatalf("shred failed: %v", err)
	}

	assertRemoved(t, path)
}

func TestShredNormalFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.txt")

	if err := os.WriteFile(
		path,
		[]byte("this is some secret data"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	if err := shred(path); err != nil {
		t.Fatalf("shred failed: %v", err)
	}

	assertRemoved(t, path)
}

func TestShredOverwritesContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	original := []byte("this is some secret data")

	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}

	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if err := overwrite(file, int64(len(original))); err != nil {
		t.Fatalf("overwrite failed: %v", err)
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	got := make([]byte, len(original))

	if _, err := io.ReadFull(file, got); err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(got, original) {
		t.Fatal("content was not overwritten")
	}
}

func TestShredRefusesHardLinks(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "victim")
	link := filepath.Join(dir, "other-name")

	original := []byte("secret data")

	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}

	if err := os.Link(path, link); err != nil {
		t.Skipf("hard links not available: %v", err)
	}

	err := shred(path)

	if !errors.Is(err, ErrMultipleNamesNotAllowed) {
		t.Fatalf(
			"expected ErrMultipleNamesNotAllowed, got: %v",
			err,
		)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("original path should still exist: %v", err)
	}

	if _, err := os.Stat(link); err != nil {
		t.Fatalf("hard link should still exist: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got, original) {
		t.Fatal("multiply-linked file content was modified")
	}
}

func TestShredOverwritesLargeFile(t *testing.T) {
	// Larger than io.Copy's 32 KiB internal buffer and not a multiple
	// of it, so the multi-chunk path and the final partial chunk are
	// both exercised.
	const size = 100*1024 + 7

	original := bytes.Repeat([]byte{0xAA}, size)

	path := filepath.Join(t.TempDir(), "large.bin")

	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}

	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if err := overwrite(file, int64(size)); err != nil {
		t.Fatalf("overwrite failed: %v", err)
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	got := make([]byte, size)

	if _, err := io.ReadFull(file, got); err != nil {
		t.Fatal(err)
	}

	const block = 4096

	for off := 0; off < size; off += block {
		end := off + block

		if end > size {
			end = size
		}

		if bytes.Equal(got[off:end], original[off:end]) {
			t.Fatalf(
				"block at offset %d was not overwritten",
				off,
			)
		}
	}

	if bytes.Equal(got, original) {
		t.Fatal("large file content was not overwritten")
	}
}

func TestShredRefusesSymlink(t *testing.T) {
	dir := t.TempDir()

	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")

	original := []byte("secret data")

	if err := os.WriteFile(target, original, 0600); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links not available: %v", err)
	}

	err := shred(link)

	if err == nil {
		t.Fatal("expected error for symbolic link")
	}

	// The symlink itself must still exist.
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("symbolic link should still exist: %v", err)
	}

	// The target must still exist.
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("target should still exist: %v", err)
	}

	// Most importantly, the target content must be untouched.
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got, original) {
		t.Fatal("target content was modified through symbolic link")
	}
}

func TestShredPathChanged(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "victim")
	oldPath := filepath.Join(dir, "victim-old")
	replacement := filepath.Join(dir, "replacement")

	originalData := []byte("original secret data")
	replacementData := []byte("replacement data")

	if err := os.WriteFile(path, originalData, 0600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		replacement,
		replacementData,
		0600,
	); err != nil {
		t.Fatal(err)
	}

	testHookAfterLstat = func() {
		if err := os.Rename(path, oldPath); err != nil {
			t.Fatalf("could not rename original file: %v", err)
		}

		if err := os.Rename(replacement, path); err != nil {
			t.Fatalf("could not replace file path: %v", err)
		}
	}

	t.Cleanup(func() {
		testHookAfterLstat = nil
	})

	err := shred(path)

	if !errors.Is(err, ErrPathChanged) {
		t.Fatalf("expected ErrPathChanged, got: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got, replacementData) {
		t.Fatal("replacement file was modified")
	}

	got, err = os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got, originalData) {
		t.Fatal("original file was modified")
	}
}

func TestShredOverwriteKeepsSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.bin")

	original := bytes.Repeat([]byte{0xAA}, 10000)

	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}

	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	before, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if err := overwrite(file, before.Size()); err != nil {
		t.Fatalf("overwrite failed: %v", err)
	}

	after, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if after.Size() != before.Size() {
		t.Fatalf(
			"file size changed from %d to %d bytes",
			before.Size(),
			after.Size(),
		)
	}
}

func TestShredSpecialFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fifo")

	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Skipf("could not create FIFO: %v", err)
	}

	err := shred(path)

	if err == nil {
		t.Fatal("expected error for special file")
	}

	if !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("expected fs.ErrInvalid, got: %v", err)
	}

	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("special file should remain untouched: %v", err)
	}
}
