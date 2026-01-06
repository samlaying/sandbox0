# SandboxVolume 设计问题清单

本文档记录 SandboxVolume 架构中已识别的设计问题和需要澄清的点。

---

## 问题 3: Procd 重启后挂载状态丢失

### 严重程度: 中

### 问题描述
- 挂载状态只存在于 Procd 内存中（`map[string]*MountContext`）
- 如果 Procd 进程重启（crash 或正常重启），挂载信息会丢失
- FUSE mount point 可能变成 "ghost mount"（FUSE 进程已死，挂载点仍存在）

### 需要的解决方案
- Procd 需要在启动时恢复挂载状态，或者
- Sandbox 删除时必须强制清理挂载点

---

## 问题 4: Snapshot Restore 期间的挂载状态

### 严重程度: 中

### 问题描述
Restore API（storage-proxy.md:275-291）只恢复 JuiceFS 数据。
如果 SandboxVolume 在 restore 期间已挂载：
- FUSE cache 是否会失效？
- 用户是否需要重新挂载才能看到恢复后的数据？

### 需要澄清
需要定义 volume 已挂载时 restore 的行为约束和通知机制。

---

## 问题 5: Packet Marking 可移植性

### 严重程度: 低 (设计限制)

### 问题描述
```go
// From client-sandboxvolume.md:495-523
syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, 0x24, 0x2)
```
- `SO_MARK` (0x24) 是 Linux 特有的 socket option
- 限制了 Sandbox0 只能在 Linux 上运行
- 不可移植到 macOS/Windows 容器平台

### 需要的行动
明确记录平台限制，或考虑跨平台支持的替代方案。

---


## 问题 7: Snapshot/Restore 性能与限制

### 严重程度: 低 (文档缺失)

### 缺失信息
- snapshot 是 JuiceFS snapshot 还是 S3 snapshot？
- 大型卷的 snapshot 需要多长时间？
- restore 是否会影响正在进行的读写操作？
- 是否有 quota 限制（snapshot 数量、存储空间）？

### 需要补充文档
在 spec 中添加性能特性和限制说明。

---

## 总结

| 问题类型 | 严重程度 | 数量 |
|---------|---------|-----|
| 需要澄清的设计决策 | 高 | 2 |
| 边界情况处理 | 中 | 2 |
| 性能优化 | 中 | 1 |
| 架构关注点 | 低 | 2 |
| 文档缺失 | 低 | 1 |

### 整体评估
架构设计是**合理且清晰的**，主要关注点在于：
1. **性能优化**（Procd本地读缓存、Node亲和性路由）

这些问题应在实现前予以澄清。
