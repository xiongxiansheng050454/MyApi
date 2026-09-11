# 下游接口 — Models

**路径**：`GET /v1/models`

返回当前网关对下游发布的模型列表（OpenAI 兼容），供下游枚举可用模型。

---

## 1. 认证

与 Chat Completions 相同：`Authorization: Bearer <网关Key>`。

- 只返回该 Key `permissions.models` 白名单内的模型；`["*"]` 表示全部。
- 列表来源为当前启用渠道且启用映射的对外模型名（`channel_models.model_name`）。

## 2. 请求

```http
GET /v1/models HTTP/1.1
Host: llm.example.com
Authorization: Bearer sk-abc123...
```

## 3. 响应

```json
{
  "object": "list",
  "data": [
    { "id": "gpt-4", "object": "model", "created": 1730000000, "owned_by": "myapi" },
    { "id": "deepseek-chat", "object": "model", "created": 1730000000, "owned_by": "myapi" }
  ]
}
```

## 4. 错误

鉴权失败与 Chat Completions 一致（`401 invalid_api_key` / `key_expired` / `key_disabled`、`403 account_suspended`），见 [chat-completions §4](chat-completions.md#4-错误下游)。
