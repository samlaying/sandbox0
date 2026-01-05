# Procd - 文件系统操作设计规范

## 一、设计目标

文件系统操作是 Procd 的核心功能之一，提供：
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
│   │  - Read / Write / Stat / List / MakeDir / Move / Remove                │  │
│   │  - WatcherManager (fsnotify)                                          │  │
│   └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 三、安全考虑

### 3.1 路径安全
- 所有路径操作限制在 `rootPath` 内
- 防止路径穿越攻击 (`../`)
- 使用 `filepath.Clean()` 和前缀检查

### 3.2 文件大小限制
- 默认最大文件大小：100MB
- 可通过环境变量配置

### 3.3 权限控制
- 可配置是否允许创建可执行文件

---

## 四、错误定义

| 错误 | 说明 |
|------|------|
| `file_not_found` | 文件不存在 |
| `directory_not_found` | 目录不存在 |
| `path_outside_root` | 路径在根目录外 |
| `file_too_large` | 文件超过大小限制 |
| `permission_denied` | 权限不足 |
| `watcher_not_found` | 监听器不存在 |

---

## 五、与 E2B 兼容性

| E2B API | Sandbox0 API | 说明 |
|---------|---------------|------|
| `Filesystem.Stat` | `GET /api/v1/files/*path?stat=true` | ✅ |
| `Filesystem.MakeDir` | `POST /api/v1/files/*path?mkdir=true` | ✅ |
| `Filesystem.Move` | `POST /api/v1/files/move` | ✅ |
| `Filesystem.Remove` | `DELETE /api/v1/files/*path` | ✅ |
| `Filesystem.ListDir` | `GET /api/v1/files/*path?list=true` | ✅ |
| `Filesystem.WatchDir` | `WS /api/v1/files/watch` | ✅ WebSocket 替代 RPC |
