# Procd - 文件系统操作设计规范

## 一、设计目标

文件系统操作是Procd的核心功能之一，提供：
1. **完整文件操作**：读、写、stat、mkdir、move、删除、列出目录
2. **目录监听**：实时监听文件系统变化
3. **WebSocket支持**：流式文件事件推送

---

## 二、架构设计

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    File Manager Architecture                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   HTTP Server                                                                │
│   ┌───────────────────────────────────────────────────────────────────────┐  │
│   │  File API Handler                                                      │  │
│   │  - GET    /api/v1/files/*path          (read/stat/list)               │  │
│   │  - POST   /api/v1/files/*path          (write/mkdir)                  │  │
│   │  - POST   /api/v1/files/move           (move)                         │  │
│   │  - DELETE /api/v1/files/*path          (remove)                       │  │
│   │  - WS     /api/v1/files/watch          (watch)                        │  │
│   └───────────────────────────────────────────────────────────────────────┘  │
│                                    │                                         │
│                                    ▼                                         │
│   ┌───────────────────────────────────────────────────────────────────────┐  │
│   │                        FileManager                                     │  │
│   │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐     │  │
│   │  │    Read     │ │    Write    │ │    Stat     │ │   Watcher   │     │  │
│   │  │   Files     │ │    Files    │ │   Files     │ │  Manager    │     │  │
│   │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘     │  │
│   └───────────────────────────────────────────────────────────────────────┘  │
│                                    │                                         │
│                                    ▼                                         │
│   ┌───────────────────────────────────────────────────────────────────────┐  │
│   │                    fsnotify (底层监听库)                               │  │
│   └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 三、数据结构定义

### 3.1 FileInfo 文件信息

```go
// FileInfo 文件信息
type FileInfo struct {
    Name      string    `json:"name"`       // 文件名
    Path      string    `json:"path"`       // 完整路径
    Type      FileType  `json:"type"`       // 文件类型
    Size      int64     `json:"size"`       // 字节数
    Mode      string    `json:"mode"`       // 权限模式 (0644)
    ModTime   time.Time `json:"mod_time"`   // 修改时间
    IsLink    bool      `json:"is_link"`    // 是否符号链接
    LinkTarget string   `json:"link_target,omitempty"` // 链接目标
}

type FileType string

const (
    FileTypeFile     FileType = "file"     // 普通文件
    FileTypeDir      FileType = "dir"      // 目录
    FileTypeSymlink  FileType = "symlink"  // 符号链接
)
```

### 3.2 WatchEvent 文件事件

```go
// WatchEvent 文件系统事件
type WatchEvent struct {
    WatchID   string     `json:"watch_id"`   // 监听ID
    Type      EventType  `json:"type"`       // 事件类型
    Path      string     `json:"path"`       // 文件路径
    OldPath   string     `json:"old_path,omitempty"` // 重命名时的原路径
    Timestamp time.Time  `json:"timestamp"`  // 事件时间
}

type EventType string

const (
    EventCreate  EventType = "create"  // 文件创建
    EventWrite   EventType = "write"   // 文件写入
    EventRemove  EventType = "remove"  // 文件删除
    EventRename  EventType = "rename"  // 文件重命名
    EventChmod   EventType = "chmod"   // 权限变更
)
```

### 3.3 Watcher 监听器

```go
// Watcher 目录监听器
type Watcher struct {
    ID        string              // 监听器ID
    Path      string              // 监听路径
    Recursive bool                // 是否递归监听
    EventChan chan<- WatchEvent   // 事件通道
    cancel    context.CancelFunc   // 取消函数
}

// WatcherManager 监听器管理器
type WatcherManager struct {
    mu       sync.RWMutex
    watchers map[string]*Watcher
    fsNotify *fsnotify.Watcher
}
```

---

## 四、FileManager 实现

### 4.1 核心接口

```go
// FileManager 文件管理器
type FileManager struct {
    mu             sync.RWMutex
    rootPath       string              // 根路径 (如 /workspace)
    watcherMgr     *WatcherManager
}

// ReadFile 读取文件
func (fm *FileManager) ReadFile(path string) ([]byte, error)

// WriteFile 写入文件
func (fm *FileManager) WriteFile(path string, data []byte, perm os.FileMode) error

// Stat 获取文件信息
func (fm *FileManager) Stat(path string) (*FileInfo, error)

// ListDir 列出目录
func (fm *FileManager) ListDir(path string) ([]*FileInfo, error)

// MakeDir 创建目录
func (fm *FileManager) MakeDir(path string, perm os.FileMode, recursive bool) error

// Move 重命名/移动文件
func (fm *FileManager) Move(src, dst string) error

// Remove 删除文件
func (fm *FileManager) Remove(path string) error

// WatchDir 监听目录
func (fm *FileManager) WatchDir(path string, recursive bool) (*Watcher, error)

// UnwatchDir 取消监听
func (fm *FileManager) UnwatchDir(watchID string) error
```

### 4.2 ReadFile 实现

```go
func (fm *FileManager) ReadFile(path string) ([]byte, error) {
    // 1. 清理路径，防止路径穿越攻击
    cleanPath := filepath.Join(fm.rootPath, path)
    if !strings.HasPrefix(cleanPath, fm.rootPath) {
        return nil, fmt.Errorf("path outside root: %s", path)
    }

    // 2. 读取文件
    data, err := os.ReadFile(cleanPath)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, ErrFileNotFound
        }
        return nil, err
    }

    return data, nil
}
```

### 4.3 WriteFile 实现

```go
func (fm *FileManager) WriteFile(path string, data []byte, perm os.FileMode) error {
    // 1. 清理路径
    cleanPath := filepath.Join(fm.rootPath, path)
    if !strings.HasPrefix(cleanPath, fm.rootPath) {
        return fmt.Errorf("path outside root: %s", path)
    }

    // 2. 确保父目录存在
    dir := filepath.Dir(cleanPath)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return err
    }

    // 3. 写入文件（原子写入）
    tmpPath := cleanPath + ".tmp"
    if err := os.WriteFile(tmpPath, data, perm); err != nil {
        return err
    }

    // 4. 重命名（原子操作）
    return os.Rename(tmpPath, cleanPath)
}
```

### 4.4 Stat 实现

```go
func (fm *FileManager) Stat(path string) (*FileInfo, error) {
    cleanPath := filepath.Join(fm.rootPath, path)
    if !strings.HasPrefix(cleanPath, fm.rootPath) {
        return nil, fmt.Errorf("path outside root: %s", path)
    }

    info, err := os.Lstat(cleanPath) // 使用Lstat以支持符号链接
    if err != nil {
        if os.IsNotExist(err) {
            return nil, ErrFileNotFound
        }
        return nil, err
    }

    fileInfo := &FileInfo{
        Name:    info.Name(),
        Path:    path,
        Size:    info.Size(),
        Mode:    fmt.Sprintf("%04o", info.Mode().Perm()),
        ModTime: info.ModTime(),
    }

    // 判断文件类型
    switch {
    case info.Mode()&os.ModeSymlink != 0:
        fileInfo.Type = FileTypeSymlink
        if target, err := os.Readlink(cleanPath); err == nil {
            fileInfo.LinkTarget = target
        }
    case info.IsDir():
        fileInfo.Type = FileTypeDir
    default:
        fileInfo.Type = FileTypeFile
    }

    return fileInfo, nil
}
```

### 4.5 MakeDir 实现

```go
func (fm *FileManager) MakeDir(path string, perm os.FileMode, recursive bool) error {
    cleanPath := filepath.Join(fm.rootPath, path)
    if !strings.HasPrefix(cleanPath, fm.rootPath) {
        return fmt.Errorf("path outside root: %s", path)
    }

    if recursive {
        return os.MkdirAll(cleanPath, perm)
    }

    // 非递归创建，需要父目录存在
    return os.Mkdir(cleanPath, perm)
}
```

### 4.6 Move 实现

```go
func (fm *FileManager) Move(src, dst string) error {
    cleanSrc := filepath.Join(fm.rootPath, src)
    cleanDst := filepath.Join(fm.rootPath, dst)

    // 路径检查
    if !strings.HasPrefix(cleanSrc, fm.rootPath) || !strings.HasPrefix(cleanDst, fm.rootPath) {
        return fmt.Errorf("path outside root")
    }

    // 确保目标目录存在
    dstDir := filepath.Dir(cleanDst)
    if err := os.MkdirAll(dstDir, 0755); err != nil {
        return err
    }

    return os.Rename(cleanSrc, cleanDst)
}
```

### 4.7 ListDir 实现

```go
func (fm *FileManager) ListDir(path string) ([]*FileInfo, error) {
    cleanPath := filepath.Join(fm.rootPath, path)
    if !strings.HasPrefix(cleanPath, fm.rootPath) {
        return nil, fmt.Errorf("path outside root: %s", path)
    }

    entries, err := os.ReadDir(cleanPath)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, ErrDirNotFound
        }
        return nil, err
    }

    var result []*FileInfo
    for _, entry := range entries {
        info, _ := entry.Info()
        fileInfo := &FileInfo{
            Name:    entry.Name(),
            Path:    filepath.Join(path, entry.Name()),
            Size:    info.Size(),
            Mode:    fmt.Sprintf("%04o", info.Mode().Perm()),
            ModTime: info.ModTime(),
        }

        // 判断类型
        if entry.IsDir() {
            fileInfo.Type = FileTypeDir
        } else if info.Mode()&os.ModeSymlink != 0 {
            fileInfo.Type = FileTypeSymlink
        } else {
            fileInfo.Type = FileTypeFile
        }

        result = append(result, fileInfo)
    }

    return result, nil
}
```

---

## 五、Watcher 实现

### 5.1 WatcherManager

```go
// WatcherManager 监听器管理器
type WatcherManager struct {
    mu           sync.RWMutex
    watchers     map[string]*Watcher
    fsNotify     *fsnotify.Watcher
    eventBroadcaster *EventBroadcaster
}

// NewWatcherManager 创建监听器管理器
func NewWatcherManager() (*WatcherManager, error) {
    fw, err := fsnotify.NewWatcher()
    if err != nil {
        return nil, err
    }

    wm := &WatcherManager{
        watchers:     make(map[string]*Watcher),
        fsNotify:     fw,
        eventBroadcaster: NewEventBroadcaster(),
    }

    // 启动事件处理循环
    go wm.eventLoop()

    return wm, nil
}

// eventLoop 处理fsnotify事件
func (wm *WatcherManager) eventLoop() {
    for {
        select {
        case event, ok := <-wm.fsNotify.Events:
            if !ok {
                return
            }
            wm.handleFsEvent(event)

        case err, ok := <-wm.fsNotify.Errors:
            if !ok {
                return
            }
            log.Printf("watcher error: %v", err)
        }
    }
}

// handleFsEvent 处理文件系统事件
func (wm *WatcherManager) handleFsEvent(event fsnotify.Event) {
    wm.mu.RLock()
    defer wm.mu.RUnlock()

    // 转换事件类型
    var eventType EventType
    switch {
    case event.Op&fsnotify.Create == fsnotify.Create:
        eventType = EventCreate
    case event.Op&fsnotify.Write == fsnotify.Write:
        eventType = EventWrite
    case event.Op&fsnotify.Remove == fsnotify.Remove:
        eventType = EventRemove
    case event.Op&fsnotify.Rename == fsnotify.Rename:
        eventType = EventRename
    case event.Op&fsnotify.Chmod == fsnotify.Chmod:
        eventType = EventChmod
    default:
        return
    }

    // 广播事件到所有匹配的监听器
    watchEvent := WatchEvent{
        Type:      eventType,
        Path:      event.Name,
        Timestamp: time.Now(),
    }

    for _, watcher := range wm.watchers {
        if wm.matchWatcher(watcher, event.Name) {
            select {
            case watcher.EventChan <- watchEvent:
            default:
                // 通道满了，丢弃事件
            }
        }
    }
}

// matchWatcher 检查事件是否匹配监听器
func (wm *WatcherManager) matchWatcher(watcher *Watcher, eventPath string) bool {
    // 精确匹配或子路径匹配
    if eventPath == watcher.Path {
        return true
    }
    if watcher.Recursive && strings.HasPrefix(eventPath, watcher.Path+string(filepath.Separator)) {
        return true
    }
    return false
}

// WatchDir 监听目录
func (wm *WatcherManager) WatchDir(path string, recursive bool) (*Watcher, error) {
    wm.mu.Lock()
    defer wm.mu.Unlock()

    // 创建监听器
    eventChan := make(chan WatchEvent, 100)
    ctx, cancel := context.WithCancel(context.Background())

    watcher := &Watcher{
        ID:        generateID("watch"),
        Path:      path,
        Recursive: recursive,
        EventChan: eventChan,
        cancel:    func() { cancel() },
    }

    // 添加到fsnotify
    if err := wm.fsNotify.Add(path); err != nil {
        return nil, err
    }

    // 如果是递归监听，添加所有子目录
    if recursive {
        filepath.Walk(path, func(walkPath string, info os.FileInfo, err error) error {
            if err != nil {
                return nil
            }
            if info.IsDir() && walkPath != path {
                wm.fsNotify.Add(walkPath)
            }
            return nil
        })
    }

    wm.watchers[watcher.ID] = watcher

    // 启动清理goroutine
    go func() {
        <-ctx.Done()
        wm.UnwatchDir(watcher.ID)
    }()

    return watcher, nil
}

// UnwatchDir 取消监听
func (wm *WatcherManager) UnwatchDir(watchID string) error {
    wm.mu.Lock()
    defer wm.mu.Unlock()

    watcher, ok := wm.watchers[watchID]
    if !ok {
        return ErrWatcherNotFound
    }

    // 从fsnotify移除
    wm.fsNotify.Remove(watcher.Path)

    // 关闭事件通道
    close(watcher.EventChan)

    delete(wm.watchers, watchID)

    return nil
}
```

### 5.2 WebSocket Handler

```go
// WatchWebSocketHandler WebSocket监听处理器
type WatchWebSocketHandler struct {
    fileManager *FileManager
}

// HandleWebSocket 处理WebSocket连接
func (h *WatchWebSocketHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
    // 升级到WebSocket
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        return
    }
    defer conn.Close()

    // 创建消息通道
    msgChan := make(chan []byte, 100)
    errChan := make(chan error, 1)

    // 读取客户端消息
    go func() {
        for {
            _, msg, err := conn.ReadMessage()
            if err != nil {
                errChan <- err
                return
            }
            msgChan <- msg
        }
    }()

    // 管理监听器
    var activeWatchers map[string]*Watcher = make(map[string]*Watcher)

    defer func() {
        // 清理所有监听器
        for _, watcher := range activeWatchers {
            h.fileManager.watcherMgr.UnwatchDir(watcher.ID)
        }
    }()

    // 事件循环
    for {
        select {
        case msg := <-msgChan:
            // 处理客户端消息
            var req WebSocketRequest
            if err := json.Unmarshal(msg, &req); err != nil {
                continue
            }

            switch req.Action {
            case "subscribe":
                // 创建监听器
                watcher, err := h.fileManager.WatchDir(req.Path, req.Recursive)
                if err != nil {
                    h.sendError(conn, err)
                    continue
                }
                activeWatchers[watcher.ID] = watcher

                // 启动事件转发
                go h.forwardEvents(conn, watcher)

            case "unsubscribe":
                // 取消监听
                if watcher, ok := activeWatchers[req.WatchID]; ok {
                    h.fileManager.watcherMgr.UnwatchDir(watcher.ID)
                    delete(activeWatchers, watcher.ID)
                }
            }

        case err := <-errChan:
            // 连接错误
            log.Printf("websocket error: %v", err)
            return
        }
    }
}

// forwardEvents 转发文件事件到WebSocket
func (h *WatchWebSocketHandler) forwardEvents(conn *websocket.Conn, watcher *Watcher) {
    for event := range watcher.EventChan {
        data, _ := json.Marshal(event)
        if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
            return
        }
    }
}

// WebSocketRequest WebSocket请求
type WebSocketRequest struct {
    Action    string `json:"action"`     // "subscribe" | "unsubscribe"
    Path      string `json:"path"`       // 监听路径
    Recursive bool   `json:"recursive"`  // 是否递归
    WatchID   string `json:"watch_id"`   // 取消订阅时使用
}
```

---

## 六、安全考虑

### 6.1 路径安全

```go
// sanitizePath 清理路径，防止路径穿越
func sanitizePath(root, path string) (string, error) {
    // 1. 清理路径
    cleanPath := filepath.Clean(filepath.Join(root, path))

    // 2. 检查是否在根路径下
    rel, err := filepath.Rel(root, cleanPath)
    if err != nil || strings.HasPrefix(rel, "..") {
        return "", fmt.Errorf("path outside root: %s", path)
    }

    return cleanPath, nil
}
```

### 6.2 文件大小限制

```go
const MaxFileSize = 100 * 1024 * 1024 // 100MB

func (fm *FileManager) WriteFile(path string, data []byte, perm os.FileMode) error {
    if len(data) > MaxFileSize {
        return fmt.Errorf("file too large: %d bytes", len(data))
    }
    // ...
}
```

### 6.3 权限检查

```go
func (fm *FileManager) checkPermission(path string, perm os.FileMode) bool {
    // 检查是否允许创建可执行文件
    if perm&0111 != 0 {
        // 可配置是否允许
        return fm.allowExecutable
    }
    return true
}
```

---

## 七、错误定义

```go
var (
    ErrFileNotFound    = errors.New("file not found")
    ErrDirNotFound     = errors.New("directory not found")
    ErrPathOutsideRoot = errors.New("path outside root")
    ErrFileTooLarge    = errors.New("file too large")
    ErrWatcherNotFound = errors.New("watcher not found")
    ErrPermissionDenied = errors.New("permission denied")
)
```

---

## 八、与 E2B 兼容性

| E2B API | Sandbox0 API | 说明 |
|---------|---------------|------|
| `Filesystem.Stat` | `GET /api/v1/files/*path?stat=true` | ✅ |
| `Filesystem.MakeDir` | `POST /api/v1/files/*path?mkdir=true` | ✅ |
| `Filesystem.Move` | `POST /api/v1/files/move` | ✅ |
| `Filesystem.Remove` | `DELETE /api/v1/files/*path` | ✅ |
| `Filesystem.ListDir` | `GET /api/v1/files/*path?list=true` | ✅ |
| `Filesystem.WatchDir` | `WS /api/v1/files/watch` | ✅ WebSocket替代RPC |
| `GET /files` | `GET /api/v1/files/*path` | ✅ |
| `POST /files` | `POST /api/v1/files/*path` | ✅ |
