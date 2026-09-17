# BLE 通信协议

本文定义 Status Deck 桌面客户端与 ESP32-S3 状态卡之间的 BLE 通信协议。

协议目标是：第一版足够简单，方便调试；后续又能扩展到更多数据源、多个页面、确认回执、分片传输和不同硬件版本。

## 角色

```text
Desktop Client  <---- BLE ---->  Status Deck Device
Central                          Peripheral
电脑端主动扫描/连接               ESP32-S3 广播/接收数据
```

- ESP32-S3 作为 BLE Peripheral，持续广播固定设备名和 Service UUID
- 桌面客户端作为 BLE Central，扫描、连接、写入状态数据
- ESP32-S3 通过 notify 回传 ACK、设备状态或错误
- 自动重连由桌面客户端完成：记住设备后，启动时扫描并主动连接

## 设备发现

默认设备名：

```text
Status Deck
```

默认 Service UUID：

```text
7f3a0001-6c21-4b7d-9d5d-1f4f2b3a9000
```

桌面客户端扫描时应优先匹配 Service UUID；如果某些平台拿不到广播 UUID，可以退回匹配设备名。

## GATT 服务

| 名称 | UUID | 方向 | 属性 | 说明 |
| --- | --- | --- | --- | --- |
| Status Deck Service | `7f3a0001-6c21-4b7d-9d5d-1f4f2b3a9000` | - | - | Status Deck 专用服务 |
| RX Characteristic | `7f3a0002-6c21-4b7d-9d5d-1f4f2b3a9000` | 桌面端 -> 设备端 | Write / Write Without Response | 桌面端写入消息 |
| TX Characteristic | `7f3a0003-6c21-4b7d-9d5d-1f4f2b3a9000` | 设备端 -> 桌面端 | Read / Notify | 设备端回传 ACK、状态和错误 |

命名按 ESP32-S3 设备端视角：

- RX：设备接收桌面客户端数据
- TX：设备发送 notify 给桌面客户端

## 传输格式

第一版使用 UTF-8 JSON。

后续如果消息过大或刷新频率很高，可以在同一套消息信封上增加压缩、分片或二进制编码。

### 消息信封

所有业务消息都应使用统一信封：

```json
{
  "v": 1,
  "id": "msg_20260916_000001",
  "type": "status.update",
  "ts": 1789538400000,
  "source": "desktop",
  "target": "device",
  "payload": {}
}
```

字段说明：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `v` | number | 是 | 协议版本，当前为 `1` |
| `id` | string | 是 | 消息 ID，用于 ACK、去重和日志排查 |
| `type` | string | 是 | 消息类型 |
| `ts` | number | 是 | Unix 毫秒时间戳 |
| `source` | string | 否 | 消息来源，例如 `desktop`、`device` |
| `target` | string | 否 | 消息目标，例如 `device`、`desktop` |
| `payload` | object | 是 | 业务数据 |

## 消息类型

### `hello`

连接建立后，桌面客户端可以先发送 `hello`，声明自身能力。

```json
{
  "v": 1,
  "id": "msg_hello_001",
  "type": "hello",
  "ts": 1789538400000,
  "source": "desktop",
  "target": "device",
  "payload": {
    "client": "status-deck",
    "clientVersion": "0.1.0",
    "protocol": 1,
    "features": ["json", "ack", "status", "heartbeat"]
  }
}
```

设备端可通过 TX notify 回 `hello.ack`。

### `status.update`

桌面客户端推送状态数据。这个消息是协议核心。

```json
{
  "v": 1,
  "id": "msg_status_001",
  "type": "status.update",
  "ts": 1789538400000,
  "source": "desktop",
  "target": "device",
  "payload": {
    "system": {
      "hostname": "MacBook-Pro",
      "os": "darwin",
      "cpu": {
        "usage": 18.5,
        "temperature": null
      },
      "memory": {
        "used": 12.4,
        "total": 32,
        "unit": "GB"
      },
      "battery": {
        "percent": 86,
        "charging": true
      }
    },
    "services": [
      {
        "id": "api",
        "label": "API",
        "status": "ok",
        "value": "86%",
        "detail": "rate limit available"
      },
      {
        "id": "server-prod",
        "label": "Prod",
        "status": "warn",
        "value": "210ms",
        "detail": "latency high"
      }
    ],
    "metrics": [
      {
        "id": "ai-usage",
        "label": "AI Usage",
        "value": 42.5,
        "unit": "%",
        "status": "ok"
      }
    ]
  }
}
```

#### 当前系统信息载荷

桌面端第一版实际发送的 `payload.system` 使用下列字段。容量统一为 bytes，
百分比为 `0-100` 的小数；设备端会解析并存入内存变量，供当前或未来页面渲染。

```json
{
  "system": {
    "memory": {
      "totalBytes": 34359738368,
      "usedBytes": 12582912000,
      "availableBytes": 21776826368,
      "usedPercent": 36.6
    },
    "disk": {
      "path": "/",
      "totalBytes": 1000000000000,
      "usedBytes": 420000000000,
      "freeBytes": 580000000000,
      "usedPercent": 42.0
    },
    "power": {
      "available": true,
      "percent": 86,
      "charging": true,
      "onBattery": false,
      "state": "charging",
      "source": "ac"
    }
  }
}
```

`power.percent`、`charging` 与 `onBattery` 在没有电池的台式机上可以省略；
`available` 仍会保留并为 `false`。今后增加 AI 用量、API 额度、服务器状态时，
在 `payload` 下新增同级字段即可，不改变 `system` 的含义。

通用状态值：

| 值 | 含义 |
| --- | --- |
| `ok` | 正常 |
| `info` | 信息 |
| `warn` | 警告 |
| `error` | 错误 |
| `offline` | 离线 |
| `unknown` | 未知 |

### `display.set`

桌面客户端直接指定屏幕页面或卡片内容。

适合后续做“万能显示卡片”，让固件不必理解每一种业务数据。

```json
{
  "v": 1,
  "id": "msg_display_001",
  "type": "display.set",
  "ts": 1789538400000,
  "payload": {
    "page": "main",
    "layout": "cards",
    "cards": [
      {
        "id": "usage",
        "title": "AI Usage",
        "value": "42%",
        "subtitle": "Today",
        "status": "ok"
      },
      {
        "id": "server",
        "title": "Prod",
        "value": "210ms",
        "subtitle": "Latency",
        "status": "warn"
      }
    ]
  }
}
```

### `heartbeat`

桌面客户端定期发送心跳，设备端可用它判断电脑端是否还在线。

```json
{
  "v": 1,
  "id": "msg_heartbeat_001",
  "type": "heartbeat",
  "ts": 1789538400000,
  "payload": {
    "interval": 5000
  }
}
```

建议心跳间隔：

```text
5s - 30s
```

### `ack`

设备端确认收到某条消息。

```json
{
  "v": 1,
  "id": "msg_ack_001",
  "type": "ack",
  "ts": 1789538400100,
  "source": "device",
  "target": "desktop",
  "payload": {
    "ref": "msg_status_001",
    "ok": true
  }
}
```

### `error`

设备端报告解析失败、消息过大或不支持的消息类型。

```json
{
  "v": 1,
  "id": "msg_error_001",
  "type": "error",
  "ts": 1789538400100,
  "source": "device",
  "target": "desktop",
  "payload": {
    "ref": "msg_status_001",
    "code": "unsupported_type",
    "message": "Unsupported message type: status.foo"
  }
}
```

常见错误码：

| 错误码 | 说明 |
| --- | --- |
| `invalid_json` | JSON 解析失败 |
| `invalid_version` | 协议版本不支持 |
| `missing_field` | 缺少必要字段 |
| `unsupported_type` | 不支持的消息类型 |
| `payload_too_large` | 消息太大 |
| `busy` | 设备端忙，稍后重试 |

## 消息大小

BLE 单次写入长度受 MTU 限制。为了第一版实现简单：

- 单条 JSON 建议控制在 `512 bytes` 以内
- 大屏页面数据建议拆成多条 `display.set` 或后续使用分片
- 桌面端应避免每秒发送大量完整状态

推荐刷新策略：

| 数据类型 | 推荐频率 |
| --- | --- |
| 本机 CPU / 内存 | 1s - 5s |
| API 用量 / 额度 | 30s - 5min |
| 服务器健康检查 | 5s - 60s |
| 心跳 | 5s - 30s |

## 分片扩展

当消息超过当前 BLE 写入能力时，可以使用 `chunk` 信封。

```json
{
  "v": 1,
  "id": "chunk_msg_status_001_0",
  "type": "chunk",
  "ts": 1789538400000,
  "payload": {
    "ref": "msg_status_001",
    "index": 0,
    "total": 3,
    "encoding": "json",
    "data": "..."
  }
}
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `ref` | 原始消息 ID |
| `index` | 当前分片序号，从 `0` 开始 |
| `total` | 分片总数 |
| `encoding` | 原始数据编码，默认 `json` |
| `data` | 当前分片内容 |

第一版可以先不实现分片，但保留 `payload_too_large` 错误，提醒桌面端缩小消息。

## 连接流程

```text
1. 设备端启动 BLE，广播 Status Deck Service UUID
2. 桌面客户端扫描 Service UUID 或设备名
3. 桌面客户端连接设备
4. 桌面客户端订阅 TX notify
5. 桌面客户端发送 hello
6. 设备端回复 hello.ack 或 ack
7. 桌面客户端开始周期性发送 heartbeat 和 status.update
8. 断线后，设备端恢复广播，桌面客户端自动重连
```

## 实时连接与保活

Status Deck 的桌面端在启动后会自动进入连接维护循环，无需用户每次手动扫描：

- 启动即扫描并主动连接；ESP32-S3 只广播，不负责连接电脑
- 连接后订阅 TX notify，ESP32-S3 可实时回传 ACK、按键事件和错误
- 连接成功立即发送 `hello`，随后默认每 10 秒发送一次 `heartbeat`
- 写入失败、底层断连或 30 秒未收到心跳时，双方都将当前会话视为失效
- 桌面端每 5 秒重新尝试扫描连接；ESP32-S3 断线后立即恢复广播

这意味着“实时”数据不需要等待心跳：`status.update` 可在数据变化时立即通过 RX
写入。心跳只负责确认双方仍活着，不承担状态刷新。

## 兼容策略

- `v` 主版本不兼容时，设备端应返回 `invalid_version`
- 新字段必须向后兼容，旧固件可以忽略不认识的字段
- 新消息类型应先通过 `hello.features` 协商
- 固件优先支持 `hello`、`status.update`、`heartbeat`、`ack`、`error`

## MVP 实现建议

第一版只需要实现：

- 设备发现
- RX 写入 JSON
- TX notify ACK
- `hello`
- `status.update`
- `heartbeat`
- 基础错误处理

暂不需要实现：

- 分片
- 压缩
- 加密业务层
- 多设备同步
- 多页面复杂布局协议
