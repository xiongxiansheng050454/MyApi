# 管理端接口 — 用量明细 · 统计 · 费用

查询请求日志（`usage_logs`）、按用户×日的汇总（`user_daily_stats`）以及渠道成本。

- 统计口径、计费公式见 [结算与限速语义](../01-下游接口/结算与限速语义.md)。
- 时间参数一律 RFC3339；支持 `timezone`（如 `+08:00`）影响日汇总的自然日切分，默认 UTC。
- 金额为字符串，token/次数为整数。

---

## 1. 请求日志（usage_logs）

### 1.1 明细列表

`GET /admin/usage-logs?user_id=1001&channel_id=2&api_key_id=5501&model=gpt-4&status=success&request_id=01JXYZ...&start_time=2026-09-01T00:00:00Z&end_time=2026-09-08T23:59:59Z&page=1&page_size=20`

| 查询参数 | 说明 |
| --- | --- |
| `user_id` / `api_key_id` / `channel_id` | 维度过滤 |
| `model` | 对外模型名精确匹配 |
| `status` | `success / error / ...` |
| `error_code` | 失败码过滤 |
| `request_id` | 按请求 ID 精确查（对账用） |
| `start_time` / `end_time` | `created_at` 范围 |
| `page` / `page_size` | 分页 |

`list` 项：

```json
{
  "id": 700123,
  "request_id": "01JXYZ...",
  "user_id": 1001,
  "api_key_id": 5501,
  "channel_id": 2,
  "channel_name": "Azure OpenAI 生产",
  "model": "gpt-4",
  "upstream_model": "gpt-4-0613",
  "input_tokens": 32,
  "output_tokens": 87,
  "cached_input_tokens": 0,
  "total_tokens": 119,
  "unit_price_input_per_1m": "30.00000000",
  "unit_price_output_per_1m": "60.00000000",
  "total_cost": "0.00621000",
  "duration_ms": 8120,
  "ttft_ms": 620,
  "status": "success",
  "error_code": null,
  "client_ip": "203.0.113.9",
  "created_at": "2026-09-08T05:00:01Z"
}
```

### 1.2 明细详情

`GET /admin/usage-logs/{logId}`

返回完整记录（含 `extra` JSON）。

---

## 2. 统计

### 2.1 用户日汇总

`GET /admin/stats/daily?user_id=1001&date_from=2026-09-01&date_to=2026-09-08&timezone=UTC&page=1&page_size=31`

按 `user_daily_stats`（`(user_id, stat_date)` 主键）查询。

| 查询参数 | 说明 |
| --- | --- |
| `user_id` | 指定用户；缺省返回全部用户的日记录 |
| `date_from` / `date_to` | `YYYY-MM-DD`（含边界） |
| `timezone` | 日切分时区（如 `UTC`、`Asia/Shanghai`） |
| `page` / `page_size` | 分页 |

`list` 项：

```json
{
  "user_id": 1001,
  "stat_date": "2026-09-08",
  "total_input_tokens": 32000,
  "total_output_tokens": 87000,
  "total_cached_input_tokens": 0,
  "total_tokens": 119000,
  "total_cost": "6.21000000",
  "request_count": 1000,
  "success_count": 990,
  "error_count": 10
}
```

### 2.2 总览（卡片）

`GET /admin/stats/overview?start_time=2026-09-01T00:00:00Z&end_time=2026-09-08T23:59:59Z`

```json
{
  "request_count": 100000,
  "success_count": 98000,
  "error_count": 2000,
  "total_tokens": 12000000,
  "total_cost": "520.00000000",
  "active_user_count": 42
}
```

### 2.3 渠道成本

`GET /admin/stats/channels?channel_id=2&start_time=...&end_time=...`

按 `channel_id` 聚合区间内请求：

```json
{
  "list": [
    {
      "channel_id": 2,
      "channel_name": "Azure OpenAI 生产",
      "request_count": 50000,
      "total_input_tokens": 1500000,
      "total_output_tokens": 4000000,
      "total_tokens": 5500000,
      "total_cost": "300.00000000",
      "success_count": 49800,
      "error_count": 200
    }
  ]
}
```

---

## 3. 常见对账用法

| 诉求 | 接口 |
| --- | --- |
| 某用户某日费用与 token | `GET /admin/stats/daily?user_id=&date_from=date_to=` |
| 某用户资金流向 | `GET /admin/users/{id}/balance-transactions`（[users-keys.md](users-keys.md)） |
| 单次调用费用溯源 | 由下游响应头 `X-Request-Id` → `GET /admin/usage-logs?request_id=` |
| 某渠道花了多少钱 | `GET /admin/stats/channels?channel_id=` |
| 全站大盘 | `GET /admin/stats/overview` |

---

## 4. 错误码补充

| code | 含义 |
| --- | --- |
| `50000` | 日志不存在 |
| `50001` | 时间范围非法（end < start 或超跨度限制） |
| `50002` | `date_from/date_to` 格式应为 `YYYY-MM-DD` |
