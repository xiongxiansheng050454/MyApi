# 管理端接口 — 上游渠道与模型映射

管理所有上游 LLM 渠道（`channels`）及"对外模型名 ↔ 上游真实模型名"映射（`channel_models`）。支持动态增删改、启停、权重调整与连通性测试。

> 请求/响应格式、分页、金额字符串约定见 [附录](../03-附录.md)。

---

## 1. 渠道（channel）

### 1.1 创建渠道

`POST /admin/channels`

```json
{
  "name": "Azure OpenAI 生产",
  "base_url": "https://xxx.openai.azure.com/openai/v1",
  "api_key": "上游明文Key（仅创建/更新时提交，不回显）",
  "auth_type": "bearer",
  "extra_config": {},
  "status": 1,
  "weight": 100,
  "priority": 0
}
```

| 字段 | 类型 | 必填 | 默认 | 说明                                                                |
| --- | --- | --- | --- |-------------------------------------------------------------------|
| `name` | string | 是 | - | 渠道名，便于管理                                                          |
| `base_url` | string | 是 | - | 上游 OpenAI 兼容 Base URL                                             |
| `api_key` | string | 是 | - | 上游鉴权密钥，网关**加密存储**                                                 |
| `auth_type` | string | 否 | `bearer` | 预留：`bearer / header / query` 等                                    |
| `extra_config` | object | 否 | `{}` | 渠道级扩展配置（自由 JSON，由渠道适配器解释，如 `{"proxy":"...","timeout_ms":120000}`） |
| `status` | int | 否 | `1` | `1` 启用 / `0` 停用                                                   |
| `weight` | int | 否 | `100` | 负载均衡权重（同优先级渠道间按比例分配）                                              |
| `priority` | int | 否 | `0` | 路由优先级（数值越大越优先）；高优先级渠道不健康时才用低优先级                                   |

响应 `data`：

```json
{
  "id": 2,
  "name": "Azure OpenAI 生产",
  "base_url": "https://xxx.openai.azure.com/openai/v1",
  "api_key_masked": "sk-****9f3a",
  "auth_type": "bearer",
  "extra_config": {},
  "status": 1,
  "weight": 100,
  "priority": 0,
  "created_at": "2026-09-08T02:00:00Z"
}
```

> `api_key` 永不明文回显，只提供掩码 `api_key_masked`（保留末 4 位）。

### 1.2 渠道列表

`GET /admin/channels?page=1&page_size=20&status=1&keyword=Azure`

| 查询参数 | 说明 |
| --- | --- |
| `status` | `1` 启用 / `0` 停用；缺省返回全部 |
| `keyword` | 按 id / name 模糊匹配 |

每项含 §1.1 的 `data` 字段。

### 1.3 渠道详情

`GET /admin/channels/{channelId}`

返回渠道信息 + 该渠道发布的模型数 `model_count`（模型明细走 §2）。

### 1.4 更新渠道

`PUT /admin/channels/{channelId}`

可更新 `name / base_url / auth_type / extra_config / weight / priority`；`api_key` 为空表示不修改，传入新值则替换（更新后立即对后续请求生效）。

### 1.5 启停渠道

`PUT /admin/channels/{channelId}/status`

```json
{ "status": 0 }
```

- `status=0`：立即从路由候选剔除，正在处理的请求正常完成。
- `status=1`：重新启用并重置该渠道熔断状态。

### 1.6 删除渠道

`DELETE /admin/channels/{channelId}`

级联删除该渠道的 `channel_models` 与 `model_pricing`；历史 `usage_logs` 保留（可审计）。存在进行中请求时建议先停用再删。

### 1.7 连通性测试

`POST /admin/channels/{channelId}/test`

网关用渠道配置发起一次最小探测请求（如 `GET {base_url}/models`），返回：

```json
{
  "ok": true,
  "latency_ms": 320,
  "error": null
}
```

失败时 `ok=false`，`error` 为原因（鉴权失败 / 超时 / 网络错误等）。测试不产生费用、不改动熔断状态。

---

## 2. 渠道模型映射（channel_model）

对外模型名在**同一渠道下唯一**；同一对外模型可跨多个渠道发布以支持负载均衡（例如 gpt-4 同时发布在渠道 2、5）。

### 2.1 添加映射

`POST /admin/channels/{channelId}/models`

```json
{
  "model_name": "gpt-4",
  "upstream_model": "gpt-4-0613",
  "enabled": true
}
```

| 字段 | 类型 | 必填 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `model_name` | string | 是 | - | 网关对外发布的模型名（下游请求使用此名） |
| `upstream_model` | string | 是 | - | 发送给上游的真实模型名 |
| `enabled` | bool | 否 | true | `false` 时该渠道不再为该模型提供路由 |

响应 `data`：映射记录（含 `id`）。

> 同一渠道重复发布相同 `model_name` 返回冲突错误 `20002`。
> 新模型上线后建议同步维护 [pricing.md](pricing.md) 中的单价，否则按 0 价计费（不推荐）。

### 2.2 模型映射列表

`GET /admin/channels/{channelId}/models?enabled=true&model_name=gpt-4`

### 2.3 更新映射

`PUT /admin/channels/{channelId}/models/{modelId}`

可更新 `upstream_model / enabled`（`model_name` 变更走删除后重建，避免误伤路由缓存）。

### 2.4 删除映射

`DELETE /admin/channels/{channelId}/models/{modelId}`

删除后该渠道不再承接该模型的请求；级联删除对应 `model_pricing`（若有）。

---

## 3. 模型/渠道全局视角

### 3.1 发布模型目录

`GET /admin/models?status=1&model_name=`

返回网关当前可用的**对外模型名**及各自可用渠道数量，便于运营核对发布情况：

```json
{
  "list": [
    { "model_name": "gpt-4", "channel_count": 2, "channels": [{"channel_id": 2, "channel_name": "Azure OpenAI 生产", "upstream_model": "gpt-4-0613"}] }
  ]
}
```

---

## 4. 错误码补充

| code | 含义 |
| --- | --- |
| `20000` | 渠道不存在 |
| `20001` | 渠道已停用，无法执行该操作 |
| `20002` | 同一渠道已存在该 `model_name` |
| `20003` | 模型映射不存在 |
| `20004` | base_url / name / api_key 为空 |
| `20005` | 渠道仍被限流规则 / 上游引用，删除前需先停用 |
