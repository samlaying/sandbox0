# Storage Proxy - Security Design

## 一、安全模型

安全架构基于信任域隔离：

- **Untrusted Zone**: Procd Pod，用户代码运行环境
- **Trusted Zone**: Storage Proxy，所有凭证存储位置

关键安全保证：
- 用户进程无法直接访问 S3/PostgreSQL 凭证
- 所有存储操作通过认证和授权验证
- 完整的操作审计和异常检测

---

## 二、Token-Based Authentication

### 2.1 Token Flow

JWT token 认证流程：

1. Manager 为 sandbox 生成 JWT token
2. Procd 在内存中存储 token（永不写磁盘）
3. RemoteFS 在每个 gRPC 调用中发送 `Authorization: Bearer <token>`
4. Storage Proxy 验证 token 并提取权限信息

### 2.2 Token Structure

Token 包含以下声明：
- `volume_id`: 目标卷 ID
- `sandbox_id`: 请求 sandbox ID
- `team_id`: 所属团队 ID
- `exp`: 过期时间 (24 小时)
- `iat`: 签发时间
- `iss`: 签发者

### 2.3 Token Lifecycle

- **生成**：Manager 在 sandbox 分配时生成 24 小时有效期 token
- **使用**：Procd RemoteFS 在每个 gRPC 调用中发送 token
- **验证**：Storage Proxy 验证 token 并提取权限信息
- **过期**：Token 过期后自动刷新
- **撤销**：Sandbox 删除时立即撤销相关 token

---

## 三、Credential Isolation

### 3.1 Credentials Inventory

系统涉及的凭证：
- **PostgreSQL Credentials**: JuiceFS 元数据存储凭证
- **S3 Credentials**: JuiceFS 数据块存储凭证
- **JWT Secret**: Token 签名密钥

### 3.2 Credential Access Control

```
┌─────────────────┬──────────┬─────┬────────────┐
│ Component       │ Postgres │ S3  │ JWT Secret │
├─────────────────┼──────────┼─────┼────────────┤
│ Procd           │    ❌    │ ❌  │     ❌     │
│ RemoteFS        │    ❌    │ ❌  │     ❌     │
│ Storage Proxy   │    ✅    │ ✅  │     ✅     │
│ Manager         │    ✅    │ ❌  │     ✅     │
│ User Code       │    ❌    │ ❌  │     ❌     │
└─────────────────┴──────────┴─────┴────────────┘
```

### 3.3 Least Privilege IAM Policy (S3)

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:DeleteObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::sandbox0-volumes",
        "arn:aws:s3:::sandbox0-volumes/teams/${aws:username}/*"
      ],
      "Condition": {
        "StringLike": {
          "s3:prefix": ["teams/${aws:username}/*"]
        }
      }
    }
  ]
}
```

---

## 四、Network Security

### 4.1 Packet Marking

nftables 规则通过 packet marking 绕过网络隔离：

- **Proxy Bypass Rule**: `mark==0x2` 的包直接 ACCEPT
- **User Traffic**: 未标记包走正常防火墙规则
- **gRPC Client**: 设置 `SO_MARK=0x2` 标记所有连接包

### 4.2 Service Mesh Integration (Optional)

支持 Istio 服务网格：
- **PeerAuthentication**: 强制 mTLS
- **AuthorizationPolicy**: 限制只有 Procd 能连接 gRPC 端口

---

## 五、Audit Logging

### 5.1 Audit Log Fields

每次操作记录：
- 时间戳、卷 ID、Sandbox ID
- 操作类型、路径、大小
- 延迟、状态、错误信息

### 5.2 Log Destinations

- stdout (Kubernetes 日志收集)
- PostgreSQL 审计表
- 外部 SIEM (可选)

### 5.3 Anomaly Detection

检测可疑模式：
- 异常高的操作频率
- 大文件快速写入
- 访问敏感路径
- 快速文件创建/删除

---

## 六、Defense in Depth

安全分层防护：

1. **Network Isolation**: nftables 网络隔离 + packet marking
2. **Authentication**: JWT token 验证
3. **Authorization**: 基于数据库的访问控制
4. **Credential Isolation**: 凭证仅在 trusted zone
5. **Audit & Monitoring**: 全操作审计 + 异常检测
6. **Runtime Security**: AppArmor + seccomp (可选)

---

## 七、Security Best Practices

### 7.1 Deployment Checklist

- [ ] 使用 IRSA 进行 S3 访问 (无静态密钥)
- [ ] 每季度轮换 JWT secret
- [ ] 启用服务间 mTLS (Istio/Linkerd)
- [ ] 限制 Storage Proxy 仅接受来自 Procd pod 的流量
- [ ] 启用 Pod Security Policies
- [ ] 扫描容器镜像漏洞
- [ ] 启用审计日志到外部 SIEM
- [ ] 设置认证失败告警
- [ ] 为 Storage Proxy 使用独立的数据库凭证
- [ ] 加密 PostgreSQL 连接

### 7.2 Incident Response

凭证泄露时的响应流程：

1. **立即行动**
   - 撤销所有 token
   - 轮换 JWT secret
   - 轮换 S3 凭证
   - 轮换 PostgreSQL 密码

2. **调查**
   - 检查审计日志
   - 识别受影响的 sandboxes/volumes
   - 检查 S3 访问日志

3. **恢复**
   - 使用新凭证重新部署
   - 重新挂载所有活跃卷
   - 生成新 token
   - 监控异常活动