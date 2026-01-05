# Procd - 进程管理设计规范

## 一、设计目标

将所有子进程抽象为统一的 `Process` 接口，所有进程都有状态，支持完整的 Shell 特性。

### 核心思想

像真实 Terminal 一样，所有操作在持久化 Shell 环境中执行，支持 `cd`、`export`、管道等完整 Shell 特性。

---

## 二、进程类型

### 2.1 Process 类型层次

```
┌─────────────────────────────────────────────────────────────────┐
│                    Process 类型层次                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Process (interface) - 所有进程都有状态                          │
│    ├─ REPL Process        代码解释器，保持变量/导入                │
│    │   ├─ Python REPL     IPython                              │
│    │   ├─ JavaScript REPL Node.js REPL                         │
│    │   ├─ TypeScript REPL ts-node REPL                         │
│    │   ├─ Ruby REPL        IRB                                  │
│    │   └─ R REPL           R                                    │
│    │                                                          │
│    └─ Shell Process       有状态Shell，像真实Terminal              │
│        ├─ Bash            /bin/bash (默认)                       │
│        ├─ Zsh             /bin/zsh                              │
│        └─ Fish            /usr/bin/fish                         │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 2.2 进程类型对比

| ProcessType | 有状态 | 交互式 | 终端(PTY) | 典型用途 |
|-------------|--------|--------|-----------|----------|
| `repl` | ✅ | ✅ | ✅ | 代码执行、数据分析 |
| `shell` | ✅ | ✅ | ✅ | 命令执行、系统操作 |

**关键区别**：
- **REPL**: 专注语言特性（变量、函数、类），语法由语言定义
- **Shell**: 专注系统操作（文件、进程、管道），语法由 Shell 定义

---

## 三、Context（上下文）

### 3.1 概念

Context 是进程的逻辑容器，提供：
- 统一的工作目录
- 共享的环境变量
- 进程生命周期管理
- 输出流管理

### 3.2 结构

```
┌─────────────────────────────────────────────────────────────────┐
│                      Context                                    │
│  ID: ctx-abc123                                                 │
│  CWD: /home/user/project                                        │
│  EnvVars: {API_KEY: "xxx", NODE_ENV: "dev"}                    │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  MainProcess (主进程)                                   │   │
│  │  Type: repl or shell                                   │   │
│  │  PID: 1234                                              │   │
│  │                                                          │   │
│  │  如果是REPL: 变量/函数/类保持状态                          │   │
│  │  如果是Shell: 目录/环境变量/子进程保持状态                  │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## 四、进程生命周期

```
┌─────────────────────────────────────────────────────────────────┐
│                     Process Lifecycle                           │
└─────────────────────────────────────────────────────────────────┘

  Created
     │
     ▼
  Starting  ←───┐
     │          │ Restart
     ▼          │
  Running  ─────┘
     │
     ├─► Stopped (正常退出)
     │
     ├─► Killed (被Kill)
     │
     └─► Crashed (崩溃)

状态转换:
- Starting → Running: 进程成功启动
- Running → Stopped: 进程正常退出(exitCode = 0)
- Running → Killed: 收到SIGKILL信号
- Running → Crashed: 进程异常退出(exitCode != 0)
- Stopped → Running: Restart操作（变量/状态会丢失，需手动恢复）
```

---

## 五、使用场景示例

### 5.1 Python 数据分析

```go
// 创建 Python REPL Context
ctx, _ := manager.CreateContext(ProcessConfig{
    Type:     "repl",
    Language: "python",
    CWD:      "/workspace",
})

// 执行代码（状态保持）
ctx.ExecuteCode("import pandas as pd")
ctx.ExecuteCode("df = pd.read_csv('data.csv')")
ctx.ExecuteCode("print(df.shape)")  // df 变量仍然存在
```

### 5.2 Node.js 开发

```go
// 创建 Node.js REPL Context
ctx, _ := manager.CreateContext(ProcessConfig{
    Type:     "repl",
    Language: "node",
    CWD:      "/workspace",
})

// 或者创建 Shell Context 执行 npm 命令
shellCtx, _ := manager.CreateContext(ProcessConfig{
    Type:     "shell",
    Language: "bash",
    CWD:      "/workspace",
})

// 像真实终端一样操作
shellCtx.ExecuteCommand("npm install")
shellCtx.ExecuteCommand("export NODE_ENV=production")
shellCtx.ExecuteCommand("npm run build")
```

### 5.3 交互式编辑

```go
// 创建 Shell Context
ctx, _ := manager.CreateContext(ProcessConfig{
    Type:     "shell",
    Language: "bash",
    CWD:      "/workspace",
})

// 启动 vim
ctx.ExecuteCommand("vim main.py")

// 通过 WebSocket 处理用户输入
for userInput := range userInputChannel {
    ctx.MainProcess.WriteInput(userInput)
}
```

---

## 六、与 E2B 的兼容性

| E2B概念 | Sandbox0对应 |
|---------|--------------|
| Sandbox | Context |
| Kernel | REPL Process |
| Commands.run() | Shell Process.ExecuteCommand() |
| PTY.create() | Shell Process (PTY默认启用) |
| Code Interpreter | REPL Process.ExecuteCode() |

---

## 七、错误定义

| 错误 | 说明 |
|------|------|
| `context_not_found` | Context 不存在 |
| `process_start_failed` | 进程启动失败 |
| `process_killed` | 进程被杀死 |
| `process_crashed` | 进程崩溃 |
| `invalid_command` | 无效命令 |
| `permission_denied` | 权限不足 |
