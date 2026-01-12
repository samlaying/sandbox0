// Package file provides file system operations for Procd.
package file

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSanitizePath tests path resolution.
func TestSanitizePath(t *testing.T) {
	tests := []struct {
		name     string
		rootPath string
		input    string
		wantAbs  bool // whether result should be absolute
	}{
		{
			name:     "relative simple file",
			rootPath: "/tmp/test",
			input:    "file.txt",
			wantAbs:  true,
		},
		{
			name:     "relative nested path",
			rootPath: "/tmp/test",
			input:    "dir1/dir2/file.txt",
			wantAbs:  true,
		},
		{
			name:     "relative path with dot",
			rootPath: "/tmp/test",
			input:    "./file.txt",
			wantAbs:  true,
		},
		{
			name:     "relative path with double dot",
			rootPath: "/tmp/test",
			input:    "../etc/passwd",
			wantAbs:  true,
		},
		{
			name:     "relative path with multiple double dots",
			rootPath: "/tmp/test",
			input:    "../../../../../etc/passwd",
			wantAbs:  true,
		},
		{
			name:     "relative path that resolves to root",
			rootPath: "/tmp/test",
			input:    "foo/..",
			wantAbs:  true,
		},
		{
			name:     "absolute path",
			rootPath: "/tmp/test",
			input:    "/etc/passwd",
			wantAbs:  true,
		},
		{
			name:     "absolute path within root",
			rootPath: "/tmp/test",
			input:    "/tmp/test/dir/file.txt",
			wantAbs:  true,
		},
		{
			name:     "empty path",
			rootPath: "/tmp/test",
			input:    "",
			wantAbs:  true,
		},
		{
			name:     "path with trailing slash",
			rootPath: "/tmp/test",
			input:    "dir/",
			wantAbs:  true,
		},
		{
			name:     "path with multiple slashes",
			rootPath: "/tmp/test",
			input:    "dir///file.txt",
			wantAbs:  true,
		},
		{
			name:     "path with current dir in middle",
			rootPath: "/tmp/test",
			input:    "dir/./file.txt",
			wantAbs:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := NewManager(tt.rootPath)
			if err != nil {
				t.Fatalf("NewManager() failed = %v", err)
			}
			defer os.RemoveAll(tt.rootPath)

			result := m.sanitizePath(tt.input)

			// Verify result is absolute
			if !filepath.IsAbs(result) {
				t.Errorf("sanitizePath() result %s is not absolute", result)
			}
		})
	}
}

// TestWriteFileSizeLimit tests that file size limit is enforced.
func TestWriteFileSizeLimit(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-file-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	m, err := NewManager(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	// Create data exactly at the limit
	dataAtLimit := make([]byte, MaxFileSize)
	dataOverLimit := make([]byte, MaxFileSize+1)

	tests := []struct {
		name    string
		data    []byte
		wantErr error
	}{
		{
			name:    "file at size limit",
			data:    dataAtLimit,
			wantErr: nil,
		},
		{
			name:    "file over size limit",
			data:    dataOverLimit,
			wantErr: ErrFileTooLarge,
		},
		{
			name:    "small file",
			data:    []byte("hello"),
			wantErr: nil,
		},
		{
			name:    "empty file",
			data:    []byte{},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.WriteFile("test.txt", tt.data, 0644)
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("WriteFile() expected error %v, got nil", tt.wantErr)
				} else if err != tt.wantErr {
					t.Errorf("WriteFile() error = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("WriteFile() unexpected error = %v", err)
			}
		})
	}
}

// TestWriteFileExecutablePermission tests that executable permission is controlled.
func TestWriteFileExecutablePermission(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-file-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name            string
		allowExecutable bool
		perm            os.FileMode
		wantErr         error
	}{
		{
			name:            "executable file when allowed",
			allowExecutable: true,
			perm:            0755,
			wantErr:         nil,
		},
		{
			name:            "executable file when not allowed",
			allowExecutable: false,
			perm:            0755,
			wantErr:         ErrPermissionDenied,
		},
		{
			name:            "non-executable file when not allowed",
			allowExecutable: false,
			perm:            0644,
			wantErr:         nil,
		},
		{
			name:            "partially executable file when not allowed",
			allowExecutable: false,
			perm:            0744,
			wantErr:         ErrPermissionDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := NewManager(tempDir)
			if err != nil {
				t.Fatal(err)
			}
			m.allowExecutable = tt.allowExecutable
			defer m.Close()

			err = m.WriteFile("test.sh", []byte("#!/bin/bash\necho test"), tt.perm)
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("WriteFile() expected error %v, got nil", tt.wantErr)
				} else if err != tt.wantErr {
					t.Errorf("WriteFile() error = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("WriteFile() unexpected error = %v", err)
			}
		})
	}
}

// TestWriteFileAtomic tests that writes are atomic (using temp file + rename).
func TestWriteFileAtomic(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-file-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	m, err := NewManager(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	filename := "test.txt"
	initialData := []byte("initial data")
	updatedData := []byte("updated data")

	// Write initial data
	err = m.WriteFile(filename, initialData, 0644)
	if err != nil {
		t.Fatalf("WriteFile() failed = %v", err)
	}

	// Read to verify
	data, err := m.ReadFile(filename)
	if err != nil {
		t.Fatalf("ReadFile() failed = %v", err)
	}
	if string(data) != string(initialData) {
		t.Fatalf("ReadFile() data = %s, want %s", string(data), string(initialData))
	}

	// Write updated data
	err = m.WriteFile(filename, updatedData, 0644)
	if err != nil {
		t.Fatalf("WriteFile() failed = %v", err)
	}

	// Read to verify
	data, err = m.ReadFile(filename)
	if err != nil {
		t.Fatalf("ReadFile() failed = %v", err)
	}
	if string(data) != string(updatedData) {
		t.Errorf("ReadFile() data = %s, want %s", string(data), string(updatedData))
	}

	// Verify .tmp file doesn't exist after successful write
	tmpPath := filepath.Join(tempDir, filename+".tmp")
	if _, err := os.Stat(tmpPath); err == nil {
		t.Errorf("Temporary file still exists: %s", tmpPath)
	} else if !os.IsNotExist(err) {
		t.Errorf("os.Stat() unexpected error = %v", err)
	}
}

// TestMakeDirValidPaths tests directory creation with various paths.
func TestMakeDirValidPaths(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-file-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	m, err := NewManager(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	tests := []struct {
		name      string
		path      string
		recursive bool
		wantErr   bool
	}{
		{
			name:      "valid directory",
			path:      "valid_dir",
			recursive: false,
			wantErr:   false,
		},
		{
			name:      "nested directory",
			path:      "parent/child",
			recursive: true,
			wantErr:   false,
		},
		{
			name:      "path with double dot stays within temp",
			path:      "subdir/../safe_dir",
			recursive: true,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.MakeDir(tt.path, 0755, tt.recursive)
			if (err != nil) != tt.wantErr {
				t.Errorf("MakeDir() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestStatSymlink tests that Stat properly handles symlinks.
func TestStatSymlink(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-file-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	m, err := NewManager(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	// Create a regular file
	err = m.WriteFile("target.txt", []byte("target content"), 0644)
	if err != nil {
		t.Fatalf("WriteFile() failed = %v", err)
	}

	// Create a symlink
	linkPath := filepath.Join(tempDir, "link.txt")
	err = os.Symlink("target.txt", linkPath)
	if err != nil {
		t.Fatalf("os.Symlink() failed = %v", err)
	}

	// Stat the symlink
	info, err := m.Stat("link.txt")
	if err != nil {
		t.Fatalf("Stat() failed = %v", err)
	}

	if info.Type != FileTypeSymlink {
		t.Errorf("Stat() Type = %s, want %s", info.Type, FileTypeSymlink)
	}
	if !info.IsLink {
		t.Errorf("Stat() IsLink = false, want true")
	}
	if info.LinkTarget != "target.txt" {
		t.Errorf("Stat() LinkTarget = %s, want target.txt", info.LinkTarget)
	}
}

// TestListDir tests directory listing.
func TestListDir(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-file-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	m, err := NewManager(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	// Create test structure
	os.MkdirAll(filepath.Join(tempDir, "subdir"), 0755)
	m.WriteFile("file1.txt", []byte("content1"), 0644)
	m.WriteFile("file2.txt", []byte("content2"), 0644)

	// List root directory
	entries, err := m.ListDir(".")
	if err != nil {
		t.Fatalf("ListDir() failed = %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("ListDir() returned %d entries, want 3", len(entries))
	}

	// Check that we have the expected entries
	names := make(map[string]bool)
	for _, entry := range entries {
		names[entry.Name] = true
	}

	if !names["file1.txt"] || !names["file2.txt"] || !names["subdir"] {
		t.Errorf("ListDir() missing expected entries, got: %v", names)
	}
}

// TestGetRootPath tests GetRootPath returns the configured root.
func TestGetRootPath(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-file-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	m, err := NewManager(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	if m.GetRootPath() != tempDir {
		t.Errorf("GetRootPath() = %s, want %s", m.GetRootPath(), tempDir)
	}
}
