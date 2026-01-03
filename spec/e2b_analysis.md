# E2B Code Interpreter Server 与 PTY 实现原理分析

## 一、Code Interpreter Server 原理

### 1.1 架构概览

```
┌─────────────────────────────────────────────────────────────────┐
│                         E2B SDK (Python)                        │
│  sandbox.run_code() ──→ HTTP POST /execute                     │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│              Code Interpreter Server (FastAPI, Port 49999)      │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  main.py - HTTP Endpoints                                 │  │
│  │    POST /execute       - 执行代码                          │  │
│  │    POST /contexts      - 创建代码上下文                    │  │
│  │    GET  /contexts      - 列出上下文                        │  │
│  │    POST /contexts/{id}/restart - 重启上下文               │  │
│  │    DELETE /contexts/{id} - 删除上下文                      │  │
│  └───────────────────────────────────────────────────────────┘  │
│                              │                                   │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  ContextWebSocket - WebSocket客户端                       │  │
│  │    - 连接到 Jupyter WebSocket:                            │  │
│  │      ws://localhost:8888/api/kernels/{id}/channels        │  │
│  │    - 实现Jupyter Messaging Protocol                       │  │
│  │    - 处理执行状态、流式输出、结果                          │  │
│  └───────────────────────────────────────────────────────────┘  │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Jupyter Server (Port 8888)                   │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  /api/sessions - 创建会话(context)                        │  │
│  │  /api/kernels/{id}/channels - WebSocket通信              │  │
│  │    - execute_request: 执行代码请求                        │  │
│  │    - stream: stdout/stderr 流式输出                       │  │
│  │    - display_data: 显示数据(图表等)                       │  │
│  │    - execute_result: 执行结果                             │  │
│  │    - status: 执行状态(busy/idle/error)                    │  │
│  └───────────────────────────────────────────────────────────┘  │
│                              │                                   │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  IPython Kernels                                           │  │
│  │    - python, javascript, typescript, r, java, bash等      │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### 1.2 核心代码流程

**创建Context** (`contexts.py:37-72`):
```python
async def create_context(client, websockets, language, cwd) -> Context:
    # 1. 调用Jupyter API创建session
    response = await client.post(f"{JUPYTER_BASE_URL}/api/sessions", json={
        "path": str(uuid.uuid4()),
        "kernel": {"name": kernel_name},  # python3, javascript, etc.
        "type": "notebook",
        "name": str(uuid.uuid4()),
    })

    # 2. 获取kernel_id作为context_id
    context_id = session_data["kernel"]["id"]

    # 3. 创建WebSocket连接到kernel
    ws = ContextWebSocket(context_id, session_id, language, cwd)
    await ws.connect()

    # 4. 设置工作目录
    await ws.change_current_directory(cwd, language)

    return Context(language=language, id=context_id, cwd=cwd)
```

**执行代码** (`main.py:71-119` + `messaging.py:288-376`):
```python
# 1. HTTP请求接收
@app.post("/execute")
async def post_execute(exec_request: ExecutionRequest):
    # 2. 确定context (默认或指定language或context_id)
    ws = websockets.get(context_id) or websockets["default"]

    # 3. 流式返回结果
    return StreamingListJsonResponse(
        ws.execute(exec_request.code, env_vars=exec_request.env_vars)
    )

# 4. WebSocket执行流程
async def execute(self, code, env_vars, access_token):
    # 构建完整代码(注入环境变量)
    complete_code = f"{env_snippet}\n{code}"

    # 发送Jupyter execute_request
    request = self._get_execute_request(message_id, complete_code, False)
    await self._ws.send(request)

    # 流式返回结果
    async for item in self._wait_for_result(message_id):
        yield item  # Stdout, Stderr, Result, Error, EndOfExecution
```

### 1.3 Jupyter Messaging Protocol

**请求格式** (`messaging.py:100-129`):
```json
{
  "header": {
    "msg_id": "uuid",
    "username": "e2b",
    "session": "session_id",
    "msg_type": "execute_request",
    "version": "5.3"
  },
  "content": {
    "code": "print('hello')",
    "silent": false,
    "store_history": true,
    "allow_stdin": false
  }
}
```

**响应消息类型** (`messaging.py:400-543`):
| msg_type | 说明 | SDK输出 |
|----------|------|---------|
| `stream` (stdout/stderr) | 标准输出/错误 | `Stdout`, `Stderr` |
| `display_data` | 显示数据(图表) | `Result(is_main_result=False)` |
| `execute_result` | 执行结果(最后一个表达式) | `Result(is_main_result=True)` |
| `error` | 执行错误 | `Error` |
| `status` (busy/idle) | 状态变化 | 触发 `EndOfExecution` |
| `execute_input` | 输入已接受 | `NumberOfExecutions` |

### 1.4 环境变量注入机制

```python
# 为不同语言生成环境变量代码
def _set_env_var_snippet(self, key: str, value: str) -> str:
    # Python:  import os; os.environ['KEY'] = 'value'
    # JS:      process.env['KEY'] = 'value'
    # R:       Sys.setenv(KEY = "value")
    # Java:    System.setProperty("KEY", "value");
    # Bash:    export KEY='value'

# 执行前注入，执行后清理
complete_code = f"{env_vars_snippet}\n{user_code}"
# ... 执行 ...
# 后台任务清理环境变量
```

### 1.5 输出类型定义

```python
class OutputType(Enum):
    STDOUT = "stdout"
    STDERR = "stderr"
    RESULT = "result"
    ERROR = "error"
    NUMBER_OF_EXECUTIONS = "number_of_executions"
    END_OF_EXECUTION = "end_of_execution"
    UNEXPECTED_END_OF_EXECUTION = "unexpected_end_of_execution"
```

---

## 二、PTY (Pseudo Terminal) 实现原理

### 2.1 架构概览

```
┌─────────────────────────────────────────────────────────────────┐
│                    E2B SDK (Python)                             │
│  sandbox.pty.create(size, on_data)                             │
│  sandbox.pty.connect(pid, on_data)                             │
│  sandbox.pty.send_stdin(pid, data)                             │
│  sandbox.pty.resize(pid, size)                                 │
│  sandbox.pty.kill(pid)                                         │
└────────────────────────────┬────────────────────────────────────┘
                             │ Connect RPC (gRPC over HTTP)
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│              Envd (Go, Port 49983)                              │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  process.Start() - 启动PTY                                │  │
│  │    - 创建Handler (handler.New)                            │  │
│  │    - 使用github.com/creack/pty启动进程                    │  │
│  │    - 返回事件流 (Start/Data/End)                          │  │
│  └───────────────────────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  process.Connect() - 连接已有PTY                          │  │
│  │  process.SendInput() - 发送stdin                          │  │
│  │  process.Update() - 调整PTY大小                            │  │
│  │  process.SendSignal() - 发送信号(kill)                    │  │
│  └───────────────────────────────────────────────────────────┘  │
│                              │                                   │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  Handler - 进程管理                                        │  │
│  │    - DataEvent: MultiplexedChannel (多路复用输出流)       │  │
│  │    - EndEvent: 进程结束事件                               │  │
│  │    - 使用pty.StartWithSize()启动                          │  │
│  └───────────────────────────────────────────────────────────┘  │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Linux PTY                                    │
│  (master/slave pseudoterminal pair)                            │
│     master (Go reads/writes)  slave (bash process)             │
└─────────────────────────────────────────────────────────────────┘
```

### 2.2 SDK端实现 (Python)

**PTY创建** (`pty.py:105-150`):
```python
async def create(
    self,
    size: PtySize,  # rows, cols
    on_data: OutputHandler[PtyOutput],
    user: Optional[Username] = None,
    cwd: Optional[str] = None,
    envs: Optional[Dict[str, str]] = None,
    timeout: Optional[float] = 60,
) -> AsyncCommandHandle:
    # 设置TERM环境变量
    envs = envs or {}
    envs["TERM"] = "xterm-256color"

    # 调用Connect RPC的Start方法
    events = self._rpc.astart(
        process_pb2.StartRequest(
            process=process_pb2.ProcessConfig(
                cmd="/bin/bash",
                args=["-i", "-l"],  # 交互式登录shell
                envs=envs,
                cwd=cwd,
            ),
            pty=process_pb2.PTY(
                size=process_pb2.PTY.Size(
                    rows=size.rows,
                    cols=size.cols
                )
            ),
        ),
        timeout=timeout,
    )

    # 获取第一个事件(start event)，返回pid
    start_event = await events.__anext__()
    pid = start_event.event.start.pid

    return AsyncCommandHandle(
        pid=pid,
        handle_kill=lambda: self.kill(pid),
        events=events,  # 继续接收后续事件
        on_pty=on_data,  # 回调处理PTY数据
    )
```

**PTY数据流处理** (在`AsyncCommandHandle`中):
```python
# events是ServerStream，持续接收:
# - ProcessEvent_Data (PTY输出)
# - ProcessEvent_Keepalive
# - ProcessEvent_End (进程结束)

async for event in events:
    if event.event.data:
        await on_data(PtyOutput(data=event.event.data.pty))
    elif event.event.end:
        # 进程结束
        self.exit_code = event.event.end.exit_code
        break
```

**发送输入到PTY** (`pty.py:77-103`):
```python
async def send_stdin(self, pid: int, data: bytes):
    await self._rpc.asend_input(
        process_pb2.SendInputRequest(
            process=process_pb2.ProcessSelector(pid=pid),
            input=process_pb2.ProcessInput(
                pty=data,  # 原始bytes写入PTY master
            ),
        )
    )
```

**调整PTY大小** (`pty.py:216-240`):
```python
async def resize(self, pid: int, size: PtySize):
    await self._rpc.aupdate(
        process_pb2.UpdateRequest(
            process=process_pb2.ProcessSelector(pid=pid),
            pty=process_pb2.PTY(
                size=process_pb2.PTY.Size(
                    rows=size.rows,
                    cols=size.cols
                )
            ),
        )
    )
```

### 2.3 后端实现 (Go - Envd)

**Handler创建** (`handler.go:64-296`):
```go
func New(...) (*Handler, error) {
    cmd := exec.CommandContext(ctx, req.GetProcess().GetCmd(), req.GetProcess().GetArgs()...)

    // 设置用户权限
    cmd.SysProcAttr = &syscall.SysProcAttr{
        CgroupFD: cgroupFD,
        Credential: &syscall.Credential{
            Uid: uid,
            Gid: gid,
        },
    }

    // 设置环境变量
    cmd.Env = formattedVars  // PATH, HOME, USER + custom envs

    // 创建多路复用通道(支持多个订阅者)
    outMultiplex := NewMultiplexedChannel[rpc.ProcessEvent_Data](outputBufferSize)

    if req.GetPty() != nil {
        // === PTY模式 ===
        // 使用github.com/creack/pty启动
        tty, err := pty.StartWithSize(cmd, &pty.Winsize{
            Cols: uint16(req.GetPty().GetSize().GetCols()),
            Rows: uint16(req.GetPty().GetSize().GetRows()),
        })

        // 启动goroutine读取PTY输出
        go func() {
            for {
                buf := make([]byte, ptyChunkSize)  // 16KB
                n, err := tty.Read(buf)

                if n > 0 {
                    // 发送到多路复用通道
                    outMultiplex.Source <- rpc.ProcessEvent_Data{
                        Data: &rpc.ProcessEvent_DataEvent{
                            Output: &rpc.ProcessEvent_DataEvent_Pty{
                                Pty: buf[:n],
                            },
                        },
                    }
                }

                if errors.Is(err, io.EOF) {
                    break
                }
            }
        }()

        h.tty = tty
    } else {
        // === 普通进程模式 ===
        stdout, _ := cmd.StdoutPipe()
        stderr, _ := cmd.StderrPipe()
        stdin, _ := cmd.StdinPipe()

        // 分别处理stdout/stderr
        go handleStdout(stdout, outMultiplex)
        go handleStderr(stderr, outMultiplex)
        h.stdin = stdin
    }

    return h, nil
}
```

**Start方法** (`handler.go:356-378`):
```go
func (p *Handler) Start() (uint32, error) {
    // PTY在New()中已经启动，这里只处理普通进程
    if p.tty == nil {
        p.cmd.Start()
    }

    // 调整OOM score(内存不足时的优先级)
    adjustOomScore(p.cmd.Process.Pid, defaultOomScore)

    return uint32(p.cmd.Process.Pid), nil
}
```

**Wait方法** (`handler.go:380-417`):
```go
func (p *Handler) Wait() {
    // 等待输出goroutine结束
    <-p.outCtx.Done()

    // 等待进程结束
    p.cmd.Wait()

    // 关闭TTY
    p.tty.Close()

    // 发送结束事件
    p.EndEvent.Source <- rpc.ProcessEvent_End{
        End: &rpc.ProcessEvent_EndEvent{
            ExitCode: int32(p.cmd.ProcessState.ExitCode()),
            Exited:   p.cmd.ProcessState.Exited(),
        },
    }
}
```

**多路复用通道** (`multiplex.go`):
```go
// 支持多个订阅者订阅同一个事件流
type MultiplexedChannel[T any] struct {
    Source chan T
    subscribers []chan T
}

func (m *MultiplexedChannel[T]) Fork() (chan T, func()) {
    sub := make(chan T, bufferSize)
    m.subscribers = append(m.subscribers, sub)

    // 分发逻辑: 从Source读取，发送到所有订阅者
    return sub, func() { cancel() }
}
```

### 2.4 Connect RPC 协议定义

```protobuf
message StartRequest {
  ProcessConfig process = 1;
  PTY pty = 2;           // PTY配置
  bool stdin = 3;
  string tag = 4;
}

message ProcessConfig {
  string cmd = 1;
  repeated string args = 2;
  map<string, string> envs = 3;
  string cwd = 4;
}

message PTY {
  Size size = 1;
}

message PTY.Size {
  uint32 rows = 1;
  uint32 cols = 2;
}

message StartResponse {
  ProcessEvent event = 1;  // 流式事件
}

message ProcessEvent {
  oneof event {
    StartEvent start = 1;
    DataEvent data = 2;
    EndEvent end = 3;
    KeepAlive keepalive = 4;
  }
}

message ProcessEvent_DataEvent {
  oneof output {
    bytes stdout = 1;
    bytes stderr = 2;
    bytes pty = 3;      // PTY原始输出
  }
}

message ProcessEvent_EndEvent {
  int32 exit_code = 1;
  bool exited = 2;
  string error = 3;
  string status = 4;
}
```

### 2.5 常量定义

```go
const (
    defaultOomScore  = 100
    outputBufferSize = 64
    stdChunkSize     = 2 << 14  // 16KB
    ptyChunkSize     = 2 << 13  // 8KB
)
```

---

## 三、关键设计要点总结

### 3.1 Code Interpreter Server

| 特性 | 实现方式 |
|------|----------|
| **多语言支持** | Jupyter多kernel (ipython, ijavascript, irkernel, etc.) |
| **代码上下文隔离** | 每个context = 一个Jupyter kernel |
| **状态保持** | Kernel进程持续运行，变量/导入跨请求保留 |
| **流式输出** | WebSocket + Server-Sent Events (SSE) |
| **环境变量** | 代码注入方式，执行后异步清理 |
| **工作目录** | 魔法命令 (`%cd`, `process.chdir`) |

### 3.2 PTY实现

| 特性 | SDK (Python) | 后端 (Go) |
|------|--------------|-----------|
| **启动** | `create(size, on_data, ...)` | `pty.StartWithSize()` |
| **连接** | `connect(pid, on_data)` | 从进程表查找Handler |
| **输入** | `send_stdin(pid, data)` | `tty.Write(data)` |
| **调整大小** | `resize(pid, size)` | `pty.Setsize(tty, winsize)` |
| **终止** | `kill(pid)` | `Process.Signal(SIGKILL)` |
| **多路复用** | 单订阅者 | `MultiplexedChannel`支持多订阅 |
| **协议** | Connect RPC (gRPC over HTTP) | Connect RPC handler |

### 3.3 文件结构

**Code Interpreter Server**:
```
e2b-code-interpreter/template/server/
├── main.py              # FastAPI应用入口
├── contexts.py          # Context创建和管理
├── messaging.py         # Jupyter WebSocket客户端
├── stream.py            # 流式响应处理
├── envs.py              # 环境变量管理
├── api/
│   └── models/          # Pydantic数据模型
└── utils/
    └── locks.py         # 并发控制
```

**PTY SDK**:
```
.venv/lib/python3.14/site-packages/e2b/sandbox_async/
├── commands/
│   ├── pty.py           # PTY操作
│   ├── command.py       # 普通命令操作
│   └── command_handle.py # 命令句柄
└── filesystem/
    └── filesystem.py    # 文件系统操作
```

**PTY Backend (Envd)**:
```
e2b-infra/packages/envd/internal/services/process/
├── service.go           # gRPC服务定义
├── start.go             # Start RPC实现
├── connect.go           # Connect RPC实现
├── handler/
│   ├── handler.go       # 进程Handler核心逻辑
│   └── multiplex.go     # 多路复用通道实现
├── signal.go            # SendSignal RPC实现
├── input.go             # SendInput RPC实现
├── update.go            # Update RPC实现
└── list.go              # List RPC实现
```

---

## 四、兼容性考虑

### 4.1 Code Interpreter兼容要点

1. **Jupyter依赖**: 需要运行Jupyter Server + 多语言kernels
2. **WebSocket协议**: 实现完整的Jupyter Messaging Protocol
3. **流式响应**: 需要SSE或类似机制支持流式输出
4. **Context生命周期**: 管理kernel的创建、重启、删除

### 4.2 PTY兼容要点

1. **Connect RPC**: 需要支持Connect RPC协议或做协议转换
2. **gRPC流式处理**: 需要支持服务端流式RPC
3. **PTY系统调用**: 需要使用pty库或类似机制
4. **多路复用**: 需要支持多个订阅者连接到同一进程
5. **Keepalive机制**: 长连接需要心跳保持

### 4.3 端口映射

| 服务 | E2B端口 | 说明 |
|------|---------|------|
| Code Interpreter | 49999 | FastAPI HTTP服务 |
| Envd | 49983 | Connect RPC服务 |
| Jupyter | 8888 | 内部WebSocket通信 |
