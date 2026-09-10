# 下游接口 — Chat Completions

**路径**：`POST /v1/chat/completions`

协议与 [OpenAI Chat Completions](https://platform.openai.com/docs/api-reference/chat) 兼容。下游请求体中声明的模型为**网关对外模型名**，实际转发到哪个上游渠道、上游真实模型名由网关决定，下游无需感知。

**同时支持**：
- 非流式（普通 JSON 响应）
- 流式（SSE，`text/event-stream`），上游逐 token 到达时网关即时透传

---

## 1. 认证

请求头：

```
Authorization: Bearer sk-xxxxxx
```

- `sk-xxxxxx` 为**网关 Key**（完整明文），由管理端在创建时一次性下发（见 [users-keys.md](../02-管理端接口/users-keys.md)）。
- Key 结构：`<前缀><密钥>`，默认前缀 `sk-`。网关按前缀快速定位密钥族，再对完整 Key 取哈希比对，**数据库永不存明文**。
- Key 归属某用户，鉴权通过后本次调用即计入该用户的用量与费用。

校验失败均返回 401（见下方错误表）。

### 1.1 需要满足的条件

| 条件 | 不满足时 |
| --- | --- |
| Key 存在且哈希匹配 | `401` invalid_api_key |
| Key 未过期（`expires_at` 为 NULL 或未到） | `401` invalid_api_key（过期） |
| Key 处于启用状态 | `401` invalid_api_key |
| 所属用户状态为 active | `403` account_suspended |
| 请求的 model 在 Key 的模型白名单内 | `403` model_not_allowed |
| 用户可用余额 ≥ 本次预估冻结额 | `429` insufficient_quota |
| 命中任何限速 / 限额规则 | `429` rate_limit_exceeded |

---

## 2. 请求

### 2.1 请求头

| Header | 必填 | 说明 |
| --- | --- | --- |
| `Authorization` | 是 | `Bearer <网关Key>` |
| `Content-Type` | 是 | `application/json` |
| `Accept` | 否 | 留空或 `text/event-stream` 均可；网关按请求体 `stream` 字段决定 |
| `X-Request-Id` | 否 | 下游自定义请求 ID；若未提供，网关生成 `usage_logs.request_id` 并在响应头回传 |

### 2.2 请求体

主体与 OpenAI 兼容，下面列常用字段；**未列出的 OpenAI 参数字段网关会原样透传给上游**（如 `temperature`、`top_p`、`tools`、`response_format` 等）。

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 网关对外模型名。Key 无白名单时须为网关已启用渠道所发布的模型 |
| `messages` | array | 是 | 对话消息数组（OpenAI 格式：`role`/`content`，可含 `name`、`tool_calls`、`tool_call_id`） |
| `stream` | bool | 否 | `true` 时以 SSE 返回，默认 `false` |
| `max_tokens` / `max_completion_tokens` | int | 否 | 输出上限；**同时作为费用预冻结的输出预算参考** |
| `stream_options` | object | 否 | `{"include_usage": true}` 时，流式响应末帧携带 `usage` |
| `stop` / `n` / `presence_penalty` / ... | - | 否 | 原样透传 |

> 网关扩展（可选，均不影响 OpenAI 兼容性）：
> | 字段 | 说明 |
> | --- | --- |
> | `user` | 透传的业务标识（写入 usage_logs.extra） |

### 2.3 请求示例（非流式）

```http
POST /v1/chat/completions HTTP/1.1
Host: llm.example.com
Authorization: Bearer sk-abc123...
Content-Type: application/json

{
  "model": "gpt-4",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "用三句话介绍 Go"}
  ],
  "stream": false,
  "max_tokens": 300
}
```

---

## 3. 响应

### 3.1 非流式响应

结构与 OpenAI 一致，`usage` 为网关按上游返回透传（`prompt_tokens` / `completion_tokens` / `total_tokens`，如上游提供缓存用量则含 `prompt_tokens_details.cached_tokens`）。

```http
HTTP/1.1 200 OK
Content-Type: application/json
X-Request-Id: 01JXYZ...

{
  "id": "chatcmpl-9f8a1b2c...",
  "object": "chat.completion",
  "created": 1730000000,
  "model": "gpt-4",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Go 是一门由 Google 开发的静态类型编程语言……"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 32,
    "completion_tokens": 87,
    "total_tokens": 119
  }
}
```

> `model` 字段：网关返回**下游请求时填写的对外模型名**，保证下游无需感知上游真实模型名。
> 响应中的 `id` 为网关转发生成；`X-Request-Id` 响应头恒有值，可用于排查与账单对账（对应 `usage_logs.request_id`）。

### 3.2 流式响应（SSE）

响应为 `Content-Type: text/event-stream`。网关收到上游事件后逐帧转发，格式与 OpenAI 一致：

```http
HTTP/1.1 200 OK
Content-Type: text/event-stream
X-Request-Id: 01JXYZ...

data: {"id":"chatcmpl-...","object":"chat.completion.chunk","created":1730000000,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}

data: {"id":"chatcmpl-...","object":"chat.completion.chunk","created":1730000000,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Go"},"finish_reason":null}]}

data: {"id":"chatcmpl-...","object":"chat.completion.chunk","created":1730000000,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: {"id":"chatcmpl-...","object":"chat.completion.chunk","created":1730000000,"model":"gpt-4","choices":[],"usage":{"prompt_tokens":32,"completion_tokens":87,"total_tokens":119}}

data: [DONE]
```

要点：

- 每个事件即 OpenAI 的 `chat.completion.chunk`；内容逐 token 透传。
- 结束帧固定为 `data: [DONE]`。
- 若下游 `stream_options.include_usage=true`，末帧（`[DONE]` 前）会包含 `usage`；未要求时网关**不主动附加 usage 帧**。
- 无论下游是否取到 usage，网关均以上游实际返回为准完成计费（见 [结算与限速语义.md](结算与限速语义.md)）。
- 若请求在流式中途失败（上游异常/超时/熔断），网关在发送 `[DONE]` 前推送一个错误事件：

```
data: {"error":{"message":"...","type":"upstream_error","param":null,"code":"upstream_error"}}
```

- **同渠道重试**：失败时会先对**同一渠道**重试 `routing.max_retries_per_channel` 次（默认 2），仍失败才按优先级换渠道。
- **重试边界**：仅当**尚未向下游写出任何内容**时可重试；一旦已输出 token，再重试会产生重复内容，故网关直接补发 `error` 事件 + `[DONE]` 终止。
- **空闲超时**：上游超过 `upstream.stream_idle_timeout_seconds`（默认 60s）无数据即判定卡死并中断。
- **下游断开**：客户端断开连接后网关立即取消上游请求，及时止损；已收到的完整块按分词估算计费。

> 注意：错误可能发生在已发送部分 token 之后，属于流式场景固有限制，下游需按自己的重试策略处理。

---

## 4. 错误（下游）

错误响应同样采用 OpenAI 兼容结构：

```json
{
  "error": {
    "message": "The request is not allowed because the API key does not have permission...",
    "type": "invalid_request_error",
    "param": null,
    "code": "invalid_api_key"
  }
}
```

### 4.1 错误码总表

| HTTP | `code` | `type` | 含义 | 触发场景 |
| --- | --- | --- | --- | --- |
| 400 | `invalid_request_error` | `invalid_request_error` | 请求体格式错误 / 字段缺失或非法 | JSON 解析失败、`model` 缺失、`messages` 为空等 |
| 401 | `invalid_api_key` | `authentication_error` | 网关 Key 无效 / 不存在 / 哈希不匹配 | 未带 Key、格式错误 |
| 401 | `key_expired` | `authentication_error` | Key 已过期 | `expires_at` 已过 |
| 401 | `key_disabled` | `authentication_error` | Key 被停用 | `is_active=false` |
| 403 | `account_suspended` | `permission_error` | 用户状态非 active | 用户被 suspended/deleted |
| 403 | `model_not_allowed` | `permission_error` | 模型不在该 Key 白名单 | Key 的 `permissions.models` 限制 |
| 404 | `model_not_found` | `invalid_request_error` | 网关无任何渠道发布该模型 / 该模型未启用 | 路由阶段未匹配到健康渠道模型 |
| 429 | `rate_limit_exceeded` | `rate_limit_error` | 命中限速（rpm/tpm/rpd/tpd/并发） | 见 [结算与限速语义.md](结算与限速语义.md) |
| 429 | `insufficient_quota` | `insufficient_quota` | 余额不足（无法完成预冻结） | 用户可用余额 < 预估费用 |
| 429 | `engine_overloaded` | `server_error` | 并发占满 / 排队超时 | 排队模式超时，见限流规则 `extras` |
| 500 | `internal_error` | `server_error` | 网关内部异常 | 兜底 |
| 502 | `upstream_error` | `server_error` | 无可用健康渠道（全部熔断）或所选渠道故障且无备选 | 见下 |
| 504 | `upstream_timeout` | `server_error` | 上游转发超时 | 网关配置的超时时间内未完成 |

### 4.2 说明

- **限速/余额不足返回体附重试信息头**（与 OpenAI 一致的可选头）：
  - `Retry-After`：建议重试秒数（限速/排队场景）。
- 所有下游错误响应均携带 `X-Request-Id`，便于对账排查。
- 上游渠道自身的错误（如 4xx 业务错误）默认**原样透传**给下游（保留上游 HTTP 状态与 `error` 结构）；仅当上游 5xx / 网络错误 / 超时触发熔断并重试到无健康渠道时，才返回网关自身的 `502 upstream_error`。

---

## 5. 上游故障切换对下游的影响

- 网关在发送首字节给下游**之前**，允许按策略在健康渠道间重试（见 [结算与限速语义.md](结算与限速语义.md) 熔断一节）。重试对下游透明。
- 流式转发一旦开始（已向下游写出事件）即不再切换渠道；中途断开按 §3.2 的流式错误事件处理。
