# 管理端接口 — 限流规则

维护通用限速/限额规则（`rate_limit_rules`）。适用于对下游的 rpm/tpm/rpd/tpd 与并发限制；Key 级覆盖（`client_api_keys.rate_limit_overrides`）优先级更高。

规则语义、生效顺序、超限响应见 [结算与限速语义](../01-下游接口/结算与限速语义.md)。

---

## 1. 字段说明

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `rule_name` | string | 规则名称，便于管理 |
| `target_type` | string | `global / user / api_key / model / channel` |
| `target_value` | string | 具体 id 或 `*` 通配（如 `"12"`、`"gpt-4"`、`"*"`） |
| `metric` | string | `rpm / tpm / rpd / tpd / concurrency` |
| `limit_value` | int | 窗口内上限 |
| `window_seconds` | int | 窗口秒数（`rpm/tpm` 常用 `60`；`rpd/tpd` 常用 `86400`） |
| `action` | string | `reject`（拒绝）/ `queue`（排队） |
| `priority` | int | 数值越小越优先（默认 0） |
| `enabled` | bool | 是否启用（默认 true） |
| `description` | string | 备注 |
| `extras` | object | 扩展 JSON：`queue` 模式配 `{"queue_timeout_seconds": 5}` 等 |

> `concurrency` 规则的 `window_seconds` 固定视为瞬时并发窗口，可传任意值（实现取在途计数）。
> `rpm/rpd` 计次，`tpm/tpd` 计 token（见语义文档 §3）。

---

## 2. 接口

### 2.1 创建规则

`POST /admin/rate-limits`

```json
{
  "rule_name": "普通用户每分钟请求上限",
  "target_type": "user",
  "target_value": "*",
  "metric": "rpm",
  "limit_value": 600,
  "window_seconds": 60,
  "action": "reject",
  "priority": 100,
  "enabled": true,
  "description": "user 维度默认 rpm",
  "extras": {}
}
```

响应 `data`：规则记录（含 `id`、`created_at/updated_at`）。

### 2.2 规则列表

`GET /admin/rate-limits?target_type=user&metric=rpm&enabled=true&page=1&page_size=20`

`list` 项为完整规则字段。

### 2.3 规则详情

`GET /admin/rate-limits/{ruleId}`

### 2.4 更新规则

`PUT /admin/rate-limits/{ruleId}`

任意字段可更新（`enabled` 置 `false` 即停用，也可单独用 `PUT .../status`）。修改**即时生效**，正在排队的请求不受影响。

### 2.5 删除规则

`DELETE /admin/rate-limits/{ruleId}`

---

## 3. 典型配置示例

```jsonc
// 全局限速（兜底）
{ "target_type": "global", "target_value": "*", "metric": "rpm",   "limit_value": 1000, "window_seconds": 60 }
{ "target_type": "global", "target_value": "*", "metric": "concurrency", "limit_value": 200, "window_seconds": 60 }

// 按用户
{ "target_type": "user", "target_value": "1001", "metric": "rpd", "limit_value": 50000, "window_seconds": 86400 }

// 按模型
{ "target_type": "model", "target_value": "gpt-4", "metric": "tpm", "limit_value": 900000, "window_seconds": 60 }

// 按渠道的出站保护（下游视角无需感知，供运营使用）
{ "target_type": "channel", "target_value": "2", "metric": "tpm", "limit_value": 800000, "window_seconds": 60 }

// 排队模式
{ "target_type": "user", "target_value": "*", "metric": "rpm", "limit_value": 1200, "window_seconds": 60,
  "action": "queue", "extras": { "queue_timeout_seconds": 3 } }
```

---

## 4. 错误码补充

| code | 含义 |
| --- | --- |
| `40000` | 规则不存在 |
| `40001` | `target_type` / `metric` 非法 |
| `40002` | `limit_value ≤ 0` 或 `window_seconds ≤ 0` |
| `40003` | `queue` 模式缺少 `extras.queue_timeout_seconds` |
