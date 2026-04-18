package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/sandbox0-ai/sandbox0/manager/procd/pkg/file"
	"github.com/sandbox0-ai/sandbox0/manager/procd/pkg/volume"
	"go.uber.org/zap"
)

type fakeFileManager struct {
	rootPath   string
	readCalls  int
	writeCalls int
	statCalls  int
	moveCalls  int
}

func (m *fakeFileManager) ReadFile(string) ([]byte, error) {
	m.readCalls++
	return []byte("local"), nil
}

func (m *fakeFileManager) WriteFile(string, []byte, os.FileMode) error {
	m.writeCalls++
	return nil
}

func (m *fakeFileManager) Stat(path string) (*file.FileInfo, error) {
	m.statCalls++
	return &file.FileInfo{Path: path}, nil
}

func (m *fakeFileManager) ListDir(string) ([]*file.FileInfo, error) {
	return []*file.FileInfo{{Path: "local"}}, nil
}

func (m *fakeFileManager) MakeDir(string, os.FileMode, bool) error {
	return nil
}

func (m *fakeFileManager) Move(string, string) error {
	m.moveCalls++
	return nil
}

func (m *fakeFileManager) Remove(string) error {
	return nil
}

func (m *fakeFileManager) SubscribeWatch(string, bool, func(file.WatchEvent)) (*file.Watcher, func() error, error) {
	return &file.Watcher{ID: "watch-1"}, func() error { return nil }, nil
}

func (m *fakeFileManager) GetRootPath() string {
	if m.rootPath == "" {
		return "/workspace"
	}
	return m.rootPath
}

type fakeVolumeResolver struct {
	paths map[string]*volume.MountedPath
}

func (r *fakeVolumeResolver) ResolveMountedPath(path string) (*volume.MountedPath, bool) {
	if r == nil {
		return nil, false
	}
	resolved, ok := r.paths[path]
	return resolved, ok
}

type staticTokenProvider string

func (p staticTokenProvider) GetInternalToken() string {
	return string(p)
}

func newTestFileHandler(manager *fakeFileManager, resolver *fakeVolumeResolver, baseURL string) *FileHandler {
	handler := NewFileHandler(manager, resolver, baseURL, 0, staticTokenProvider("storage-token"), zap.NewNop())
	return handler
}

func TestFileHandlerStatProxiesMountedVolumePath(t *testing.T) {
	storageProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if got := r.Header.Get("X-Internal-Token"); got != "storage-token" {
			t.Fatalf("X-Internal-Token = %q, want %q", got, "storage-token")
		}
		if got := r.URL.Path; got != "/sandboxvolumes/vol-1/files/stat" {
			t.Fatalf("path = %q, want %q", got, "/sandboxvolumes/vol-1/files/stat")
		}
		if got := r.URL.Query().Get("path"); got != "/notes.txt" {
			t.Fatalf("query path = %q, want %q", got, "/notes.txt")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"path":"/notes.txt","type":"file"}`)
	}))
	defer storageProxy.Close()

	manager := &fakeFileManager{rootPath: "/workspace"}
	handler := newTestFileHandler(manager, &fakeVolumeResolver{
		paths: map[string]*volume.MountedPath{
			"/workspace/data/notes.txt": {
				SandboxVolumeID: "vol-1",
				MountPoint:      "/workspace/data",
				RelativePath:    "/notes.txt",
			},
		},
	}, storageProxy.URL)
	handler.httpClient = storageProxy.Client()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/stat?path=/workspace/data/notes.txt", nil)
	recorder := httptest.NewRecorder()

	handler.Stat(recorder, req)

	if manager.statCalls != 0 {
		t.Fatalf("local stat calls = %d, want 0", manager.statCalls)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := strings.TrimSpace(recorder.Body.String()); got != `{"path":"/notes.txt","type":"file"}` {
		t.Fatalf("body = %q", got)
	}
}

func TestFileHandlerHandlePostProxiesMountedVolumeWrite(t *testing.T) {
	storageProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.URL.Path; got != "/sandboxvolumes/vol-1/files" {
			t.Fatalf("path = %q, want %q", got, "/sandboxvolumes/vol-1/files")
		}
		if got := r.URL.Query().Get("path"); got != "/notes.txt" {
			t.Fatalf("query path = %q, want %q", got, "/notes.txt")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if got := string(body); got != "hello" {
			t.Fatalf("body = %q, want %q", got, "hello")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"written":true}`)
	}))
	defer storageProxy.Close()

	manager := &fakeFileManager{rootPath: "/workspace"}
	handler := newTestFileHandler(manager, &fakeVolumeResolver{
		paths: map[string]*volume.MountedPath{
			"/workspace/data/notes.txt": {
				SandboxVolumeID: "vol-1",
				MountPoint:      "/workspace/data",
				RelativePath:    "/notes.txt",
			},
		},
	}, storageProxy.URL)
	handler.httpClient = storageProxy.Client()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files?path=/workspace/data/notes.txt", strings.NewReader("hello"))
	recorder := httptest.NewRecorder()

	handler.Handle(recorder, req)

	if manager.writeCalls != 0 {
		t.Fatalf("local write calls = %d, want 0", manager.writeCalls)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestFileHandlerMoveProxiesWhenPathsShareMountedVolume(t *testing.T) {
	storageProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/sandboxvolumes/vol-1/files/move" {
			t.Fatalf("path = %q, want %q", got, "/sandboxvolumes/vol-1/files/move")
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if payload["source"] != "/src.txt" || payload["destination"] != "/dst.txt" {
			t.Fatalf("payload = %+v", payload)
		}
		_, _ = io.WriteString(w, `{"moved":true}`)
	}))
	defer storageProxy.Close()

	manager := &fakeFileManager{rootPath: "/workspace"}
	handler := newTestFileHandler(manager, &fakeVolumeResolver{
		paths: map[string]*volume.MountedPath{
			"/workspace/data/src.txt": {
				SandboxVolumeID: "vol-1",
				MountPoint:      "/workspace/data",
				RelativePath:    "/src.txt",
			},
			"/workspace/data/dst.txt": {
				SandboxVolumeID: "vol-1",
				MountPoint:      "/workspace/data",
				RelativePath:    "/dst.txt",
			},
		},
	}, storageProxy.URL)
	handler.httpClient = storageProxy.Client()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/move", strings.NewReader(`{"source":"/workspace/data/src.txt","destination":"/workspace/data/dst.txt"}`))
	recorder := httptest.NewRecorder()

	handler.Move(recorder, req)

	if manager.moveCalls != 0 {
		t.Fatalf("local move calls = %d, want 0", manager.moveCalls)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestFileHandlerMoveRejectsMixedMountedAndLocalPaths(t *testing.T) {
	manager := &fakeFileManager{rootPath: "/workspace"}
	handler := newTestFileHandler(manager, &fakeVolumeResolver{
		paths: map[string]*volume.MountedPath{
			"/workspace/data/src.txt": {
				SandboxVolumeID: "vol-1",
				MountPoint:      "/workspace/data",
				RelativePath:    "/src.txt",
			},
		},
	}, "http://storage-proxy.local")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/move", strings.NewReader(`{"source":"/workspace/data/src.txt","destination":"/workspace/local/dst.txt"}`))
	recorder := httptest.NewRecorder()

	handler.Move(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if manager.moveCalls != 0 {
		t.Fatalf("local move calls = %d, want 0", manager.moveCalls)
	}
}

func TestFileHandlerStatFallsBackToLocalFileManagerForNonMountedPath(t *testing.T) {
	manager := &fakeFileManager{rootPath: "/workspace"}
	handler := newTestFileHandler(manager, &fakeVolumeResolver{paths: map[string]*volume.MountedPath{}}, "http://storage-proxy.local")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/stat?path=/workspace/local.txt", nil)
	recorder := httptest.NewRecorder()

	handler.Stat(recorder, req)

	if manager.statCalls != 1 {
		t.Fatalf("local stat calls = %d, want 1", manager.statCalls)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}
