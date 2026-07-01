# 设备绑定协议 🔗

本文描述设备身份协议：设备码设计、enroll 流程与安全取舍。

> 本文聚焦"设备码 + enroll"。绑定之后的 **Agent WebSocket 连接/状态**见
> [agent-ws.md](agent-ws.md)；**白名单命令通道**见 [commands.md](commands.md)；
> **HTTPS Pull 兜底**见 [agent-pull.md](agent-pull.md)。
>
> 边界提醒：身份、连接、命令与 Pull 通道均**不让 cloud 直接控制系统**——cloud 只投递白名单 action 的
> JSON，【没有】远程 shell、【没有】拼接 mgate.sh；真正调用 mgate.sh 是 mgate-agent 的职责。

## 设备生命周期

```
管理员创建            生成一次性设备码           agent enroll
   │                      │                         │
   ▼                      ▼                         ▼
[pending] ───────────► [pending] ───────────► [enabled]
   │  (仅 pending 可生成设备码)                     │
   │                                               │
   └──────────────── disable ◄────────────────────┘
                        │
                     [disabled]  ──enable──►  有 active 凭证→enabled / 否则→pending
```

- `pending`：已创建未绑定，可生成设备码。
- `enabled`：已绑定，拥有有效长期凭证。
- `disabled`：已禁用，拒绝 agent enroll；不可生成设备码。
- `deleted`：软删除（暂不提供入口，预留）。

## 一次性设备码设计

设备码格式：

```
mgate1.<base64url(payload-json)>.<base64url(hmac-sha256)>
```

payload 解码后形如：

```json
{
  "v": 1,
  "gateway": "https://cloud.example.com",
  "pairing_token": "mgpair_xxxxxxxx",
  "expires_at": "2026-06-23T12:00:00Z"
}
```

### 为什么设备码不包含永久密钥？

这是本设计的核心安全取舍：

- 设备码用于**首次绑定的临时凭据**，可能被打印、截图、通过聊天工具传递，泄露面较大。
- 因此它只携带**一次性** `pairing_token`，且有**过期时间**；绝不包含长期 `device_token`。
- 即便设备码泄露，攻击者最多在有效期内、且在合法 agent 抢先绑定之前，尝试**一次**绑定；
  一旦被任何一方使用，设备码立即失效（`used_at`）。
- 真正的长期凭证 `device_token` 只在 **enroll 成功响应**里返回一次，且 cloud 只保存其哈希。

### 签名与存储

- 签名：`HMAC-SHA256(MGATE_APP_SECRET, "mgate1." + base64url(payload))`，防篡改。
  改动 gateway / token / 过期时间都会导致校验失败。
- 存储：数据库 `device_pairing_codes.code_hash` 只保存 `SHA-256(pairing_token)`，
  设备码本体与明文 token 绝不落库、绝不进日志或审计 metadata。
- 比较：签名与令牌哈希比较均使用**恒定时间比较**，规避计时侧信道。

## Enroll API

```
POST /api/agent/enroll        # 无需管理员 session，无需 CSRF，靠设备码鉴权
Content-Type: application/json
```

请求体：

```json
{
  "device_code": "mgate1.xxx.yyy",
  "agent_version": "0.1.0",
  "device_info": {
    "hostname": "mgate-ufi-001",
    "model": "ufi",
    "mgate_version": "0.3.7",
    "firmware_info": "debian"
  }
}
```

成功响应：

```json
{
  "ok": true,
  "data": {
    "device_id": "dev_xxx",
    "device_token": "mgdt_xxx",
    "gateway": "https://cloud.example.com",
    "ws_url": "wss://cloud.example.com/api/agent/ws",
    "pull_url": "https://cloud.example.com/api/agent/pull"
  }
}
```

> `ws_url` / `pull_url` 是返回给 agent 的连接地址，供其后续接入
> **Agent WebSocket** 与 **HTTPS Pull 兜底**通道。

失败响应：

```json
{ "ok": false, "error": { "code": "invalid_pairing_code", "message": "设备码无效或已过期" } }
```

错误 code：

| code | 含义 |
|------|------|
| `invalid_pairing_code` | 格式错误 / 签名不符 / 查无此码（结构性失败统一归此，防枚举） |
| `expired_pairing_code` | 设备码已过期 |
| `used_pairing_code` | 设备码已被使用 |
| `device_disabled` | 设备已被禁用 |
| `device_already_enrolled` | 设备已完成绑定 |
| `invalid_request` | 请求体非法（解析失败 / 缺字段 / 超大） |
| `internal_error` | 服务端内部错误 |

### 处理流程（事务化）

enroll 在**单个数据库事务**内完成，保证原子与并发安全：

1. 离线校验设备码签名并解析载荷（不触库）。
2. 按 `SHA-256(pairing_token)` 查设备码记录。
3. 校验：未使用、未过期；设备未禁用、未绑定。
4. **并发闸门**：`UPDATE ... SET used_at WHERE code_hash=? AND used_at IS NULL`，
   仅当受影响行数为 1 才继续——保证同一设备码并发提交只有一个成功。
5. 生成长期 `device_token`，写入凭证（仅哈希）。
6. 把设备置为 `enabled`，写入自述信息与 `last_enrolled_at`。
7. 提交事务。

### curl 测试

```bash
# 1) 管理员登录拿到会话与 CSRF（略，见 README）
# 2) 创建设备、生成设备码，复制返回的 device_code
# 3) 用设备码 enroll：
curl -sS https://cloud.example.com/api/agent/enroll \
  -H 'Content-Type: application/json' \
  -d '{
    "device_code": "mgate1.xxx.yyy",
    "agent_version": "0.1.0",
    "device_info": {
      "hostname": "mgate-ufi-001",
      "model": "ufi",
      "mgate_version": "0.3.7",
      "firmware_info": "debian"
    }
  }'
```

## 安全要点小结

- `pairing_token` / `device_token` 均由 `crypto/rand` 生成（默认 256-bit），入库前哈希。
- 令牌长度足够，**不**设计为 6 位数字短码；强度由配置控制但有安全下限，不可弱化。
- 设备码一次性、会过期、不含永久密钥、签名防篡改。
- enroll 请求体大小受限（16 KiB），严格 JSON 解析（拒绝未知字段）。
- 日志与审计绝不出现设备码、pairing token、device token、cookie、session/CSRF token、口令。
