package handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/sandbox0-ai/sandbox0/manager/procd/pkg/file"
	"github.com/sandbox0-ai/sandbox0/manager/procd/pkg/volume"
	"github.com/sandbox0-ai/sandbox0/pkg/proxy"
	"go.uber.org/zap"
)

type fileManager interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	Stat(path string) (*file.FileInfo, error)
	ListDir(path string) ([]*file.FileInfo, error)
	MakeDir(path string, perm os.FileMode, recursive bool) error
	Move(src, dst string) error
	Remove(path string) error
	SubscribeWatch(path string, recursive bool, handler func(file.WatchEvent)) (*file.Watcher, func() error, error)
	GetRootPath() string
}

type mountedPathResolver interface {
	ResolveMountedPath(path string) (*volume.MountedPath, bool)
}

type internalTokenProvider interface {
	GetInternalToken() string
}

// FileHandler handles file-related HTTP requests.
type FileHandler struct {
	manager             fileManager
	volumeResolver      mountedPathResolver
	storageProxyBaseURL string
	storageProxyPort    int
	tokenProvider       internalTokenProvider
	httpClient          *http.Client
	logger              *zap.Logger
	upgrader            websocket.Upgrader
}

// NewFileHandler creates a new file handler.
func NewFileHandler(manager fileManager, volumeResolver mountedPathResolver, storageProxyBaseURL string, storageProxyPort int, tokenProvider internalTokenProvider, logger *zap.Logger) *FileHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &FileHandler{
		manager:             manager,
		volumeResolver:      volumeResolver,
		storageProxyBaseURL: storageProxyBaseURL,
		storageProxyPort:    storageProxyPort,
		tokenProvider:       tokenProvider,
		httpClient:          &http.Client{},
		logger:              logger,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

// Handle handles file operations based on HTTP method and query parameters.
func (h *FileHandler) Handle(w http.ResponseWriter, r *http.Request) {
	// Extract path from query
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "path is required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.handleGet(w, r, path)
	case http.MethodPost:
		h.handlePost(w, r, path)
	case http.MethodDelete:
		h.handleDelete(w, r, path)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

func (h *FileHandler) handleGet(w http.ResponseWriter, r *http.Request, path string) {
	query := r.URL.Query()
	if query.Has("stat") || query.Has("list") {
		writeError(w, http.StatusBadRequest, "invalid_request", "stat/list queries are not supported")
		return
	}
	if h.tryProxyMountedPath(w, r, path, "files") {
		return
	}

	// Read file
	data, err := h.manager.ReadFile(path)
	if err != nil {
		h.handleFileError(w, err)
		return
	}

	if acceptsJSON(r) {
		writeJSON(w, http.StatusOK, map[string]string{
			"content":  base64.StdEncoding.EncodeToString(data),
			"encoding": "base64",
		})
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(data)
}

func (h *FileHandler) Stat(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "path is required")
		return
	}
	if h.tryProxyMountedPath(w, r, path, "files/stat") {
		return
	}

	info, err := h.manager.Stat(path)
	if err != nil {
		h.handleFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (h *FileHandler) List(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "path is required")
		return
	}
	if h.tryProxyMountedPath(w, r, path, "files/list") {
		return
	}

	entries, err := h.manager.ListDir(path)
	if err != nil {
		h.handleFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"entries": entries,
	})
}

func acceptsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		return true
	}
	contentType := r.Header.Get("Content-Type")
	return strings.Contains(contentType, "application/json")
}

func (h *FileHandler) handlePost(w http.ResponseWriter, r *http.Request, path string) {
	if h.tryProxyMountedPath(w, r, path, "files") {
		return
	}
	query := r.URL.Query()

	if query.Get("mkdir") == "true" {
		// Create directory
		recursive := query.Get("recursive") == "true"
		if err := h.manager.MakeDir(path, 0755, recursive); err != nil {
			h.handleFileError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]bool{"created": true})
		return
	}

	// Write file
	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read_failed", err.Error())
		return
	}

	if err := h.manager.WriteFile(path, data, 0644); err != nil {
		h.handleFileError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"written": true})
}

func (h *FileHandler) handleDelete(w http.ResponseWriter, r *http.Request, path string) {
	if h.tryProxyMountedPath(w, r, path, "files") {
		return
	}
	if err := h.manager.Remove(path); err != nil {
		h.handleFileError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// Move handles file/directory move operations.
func (h *FileHandler) Move(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if req.Source == "" || req.Destination == "" {
		writeError(w, http.StatusBadRequest, "invalid_paths", "source and destination are required")
		return
	}

	srcMounted, srcOK := h.resolveMountedPath(req.Source)
	dstMounted, dstOK := h.resolveMountedPath(req.Destination)
	switch {
	case srcOK && dstOK && srcMounted.SandboxVolumeID == dstMounted.SandboxVolumeID:
		payload := map[string]string{
			"source":      srcMounted.RelativePath,
			"destination": dstMounted.RelativePath,
		}
		body, err := json.Marshal(payload)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "encode_failed", err.Error())
			return
		}
		if err := h.proxyMountedVolumeRequest(w, r, srcMounted.SandboxVolumeID, "files/move", nil, bytes.NewReader(body)); err != nil {
			h.handleProxyError(w, err)
		}
		return
	case srcOK || dstOK:
		writeError(w, http.StatusBadRequest, "invalid_paths", "cross-volume or mixed local-volume moves are not supported")
		return
	}

	if err := h.manager.Move(req.Source, req.Destination); err != nil {
		h.handleFileError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"moved": true})
}

// Watch handles WebSocket file watching.
func (h *FileHandler) Watch(w http.ResponseWriter, r *http.Request) {
	if err := proxy.DisableResponseDeadlines(w); err != nil {
		h.logger.Debug("Failed to disable file watch response deadlines", zap.Error(err))
	}
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	// Active watchers for this connection
	type watchSubscription struct {
		watcher     *file.Watcher
		unsubscribe func() error
	}
	watchers := make(map[string]watchSubscription)

	defer func() {
		// Cleanup all watchers on disconnect
		for _, watcher := range watchers {
			if watcher.unsubscribe != nil {
				_ = watcher.unsubscribe()
			}
		}
	}()

	// Read messages loop
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var req struct {
			Action    string `json:"action"`
			Path      string `json:"path"`
			Recursive bool   `json:"recursive"`
			WatchID   string `json:"watch_id"`
		}
		if err := json.Unmarshal(msg, &req); err != nil {
			continue
		}

		switch req.Action {
		case "subscribe":
			watcher, unsubscribe, err := h.manager.SubscribeWatch(req.Path, req.Recursive, func(event file.WatchEvent) {
				conn.WriteJSON(map[string]any{
					"type":     "event",
					"watch_id": event.WatchID,
					"event":    string(event.Type),
					"path":     event.Path,
				})
			})
			if err != nil {
				conn.WriteJSON(map[string]any{
					"type":  "error",
					"error": err.Error(),
				})
				continue
			}

			watchers[watcher.ID] = watchSubscription{
				watcher:     watcher,
				unsubscribe: unsubscribe,
			}

			// Send subscription confirmation
			conn.WriteJSON(map[string]any{
				"type":     "subscribed",
				"watch_id": watcher.ID,
				"path":     req.Path,
			})

		case "unsubscribe":
			if watcher, ok := watchers[req.WatchID]; ok {
				if watcher.unsubscribe != nil {
					_ = watcher.unsubscribe()
				}
				delete(watchers, req.WatchID)

				conn.WriteJSON(map[string]any{
					"type":     "unsubscribed",
					"watch_id": req.WatchID,
				})
			}
		}
	}
}

func (h *FileHandler) handleFileError(w http.ResponseWriter, err error) {
	switch err {
	case file.ErrFileNotFound:
		writeError(w, http.StatusNotFound, "file_not_found", err.Error())
	case file.ErrDirNotFound:
		writeError(w, http.StatusNotFound, "directory_not_found", err.Error())
	case file.ErrFileTooLarge:
		writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", err.Error())
	case file.ErrPermissionDenied:
		writeError(w, http.StatusForbidden, "permission_denied", err.Error())
	case file.ErrPathAlreadyExists:
		writeError(w, http.StatusConflict, "path_exists", err.Error())
	case file.ErrPathNotDir:
		writeError(w, http.StatusConflict, "path_not_directory", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "operation_failed", err.Error())
	}
}

func (h *FileHandler) tryProxyMountedPath(w http.ResponseWriter, r *http.Request, requestPath, endpoint string) bool {
	mounted, ok := h.resolveMountedPath(requestPath)
	if !ok {
		return false
	}
	queryParams := r.URL.Query()
	queryParams.Set("path", mounted.RelativePath)
	if err := h.proxyMountedVolumeRequest(w, r, mounted.SandboxVolumeID, endpoint, queryParams, r.Body); err != nil {
		h.handleProxyError(w, err)
	}
	return true
}

func (h *FileHandler) resolveMountedPath(requestPath string) (*volume.MountedPath, bool) {
	if h.volumeResolver == nil || requestPath == "" {
		return nil, false
	}
	cleanPath := filepath.Clean(requestPath)
	if !filepath.IsAbs(cleanPath) {
		root := "/"
		if h.manager != nil && h.manager.GetRootPath() != "" {
			root = h.manager.GetRootPath()
		}
		cleanPath = filepath.Clean(filepath.Join(root, cleanPath))
	}
	return h.volumeResolver.ResolveMountedPath(cleanPath)
}

func (h *FileHandler) proxyMountedVolumeRequest(w http.ResponseWriter, r *http.Request, volumeID, endpoint string, queryParams url.Values, body io.Reader) error {
	if volumeID == "" {
		return volume.ErrVolumeNotMounted
	}
	targetURL, err := h.storageProxyURL(volumeID, endpoint, queryParams)
	if err != nil {
		return err
	}
	token := ""
	if h.tokenProvider != nil {
		token = strings.TrimSpace(h.tokenProvider.GetInternalToken())
	}
	if token == "" {
		return volume.ErrMissingInternalToken
	}

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL.String(), body)
	if err != nil {
		return err
	}
	copyProxyHeaders(outReq.Header, r.Header)
	outReq.Header.Set("X-Internal-Token", token)
	outReq.Host = targetURL.Host

	resp, err := h.httpClient.Do(outReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	copyProxyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, copyErr := io.Copy(w, resp.Body)
	return copyErr
}

func (h *FileHandler) storageProxyURL(volumeID, endpoint string, queryParams url.Values) (*url.URL, error) {
	baseURL := strings.TrimSpace(h.storageProxyBaseURL)
	if baseURL == "" {
		return nil, volume.ErrStorageProxyUnavailable
	}
	if !strings.Contains(baseURL, "://") {
		baseURL = "http://" + baseURL
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	if u.Host == "" && u.Path != "" {
		u.Host = u.Path
		u.Path = ""
	}
	if h.storageProxyPort > 0 {
		host := u.Hostname()
		if host == "" {
			host = u.Host
		}
		u.Host = net.JoinHostPort(host, strconv.Itoa(h.storageProxyPort))
	}
	u.Path = path.Join("/", strings.TrimPrefix(u.Path, "/"), "sandboxvolumes", volumeID, endpoint)
	values := url.Values{}
	for key, entries := range queryParams {
		for _, value := range entries {
			values.Add(key, value)
		}
	}
	u.RawQuery = values.Encode()
	return u, nil
}

func (h *FileHandler) handleProxyError(w http.ResponseWriter, err error) {
	switch err {
	case volume.ErrStorageProxyUnavailable:
		writeError(w, http.StatusServiceUnavailable, "storage_proxy_unavailable", err.Error())
	case volume.ErrMissingInternalToken:
		writeError(w, http.StatusUnauthorized, "missing_internal_token", err.Error())
	case volume.ErrVolumeNotMounted:
		writeError(w, http.StatusNotFound, "volume_not_mounted", err.Error())
	default:
		var proxyErr *url.Error
		if errors.As(err, &proxyErr) {
			writeError(w, http.StatusBadGateway, "storage_proxy_request_failed", proxyErr.Error())
			return
		}
		writeError(w, http.StatusBadGateway, "storage_proxy_request_failed", err.Error())
	}
}

func copyProxyHeaders(dst, src http.Header) {
	for key, values := range src {
		switch {
		case strings.EqualFold(key, "Connection"),
			strings.EqualFold(key, "Content-Length"),
			strings.EqualFold(key, "Keep-Alive"),
			strings.EqualFold(key, "Proxy-Authenticate"),
			strings.EqualFold(key, "Proxy-Authorization"),
			strings.EqualFold(key, "TE"),
			strings.EqualFold(key, "Trailer"),
			strings.EqualFold(key, "Transfer-Encoding"),
			strings.EqualFold(key, "Upgrade"):
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
