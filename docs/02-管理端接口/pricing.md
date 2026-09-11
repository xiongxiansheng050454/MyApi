# 管理端接口 — 渠道×模型单价（Pricing）

维护每个「渠道 × 对外模型」的计费单价（`model_pricing`），单位：**美元 / 1M tokens**。单价仅能挂在已存在的 `channel_model` 映射上（引用约束），且同一渠道同一模型唯一。

> 金额一律字符串；格式见 [附录](../03-附录.md)。

---

## 1. 核心概念

| 字段 | 说明 |
| --- | --- |
| `input_price_per_1m` | 输入单价（$ / 1M tokens） |
| `output_price_per_1m` | 输出单价（$ / 1M tokens） |
| `cached_input_price_per_1m` | Prompt Caching 命中输入价；`null` 表示不区分缓存，缓存 token 按输入价计 |
| `currency` | 货币，默认 `USD` |

计费公式与缓存命中语义见 [结算与限速语义](../01-下游接口/结算与限速语义.md)。

---

## 2. 接口

### 2.1 设置 / 覆盖单价（Upsert）

`POST /admin/pricing`

同一 `(channel_id, model_name)` 已存在则覆盖更新，否则创建：

```json
{
  "channel_id": 2,
  "model_name": "gpt-4",
  "input_price_per_1m": "30.00000000",
  "output_price_per_1m": "60.00000000",
  "cached_input_price_per_1m": "15.00000000",
  "currency": "USD"
}
```

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `channel_id` | 是 | 渠道必须存在 |
| `model_name` | 是 | 该渠道必须发布过此模型（否则 `20002/20003` 类错误） |
| `input_price_per_1m` | 是 | ≥ 0，最多 8 位小数 |
| `output_price_per_1m` | 是 | ≥ 0，最多 8 位小数 |
| `cached_input_price_per_1m` | 否 | 缺省时 `= input_price_per_1m` |
| `currency` | 否 | 默认 `USD` |

响应 `data`：该定价记录（含 `id`、`created_at/updated_at`）。

### 2.2 查询单价

`GET /admin/pricing?channel_id=2&model_name=gpt-4&page=1&page_size=20`

| 查询参数 | 说明 |
| --- | --- |
| `channel_id` | 过滤渠道 |
| `model_name` | 模糊过滤模型 |
| `page` / `page_size` | 分页 |

`list` 项：

```json
{
  "id": 11,
  "channel_id": 2,
  "channel_name": "Azure OpenAI 生产",
  "model_name": "gpt-4",
  "upstream_model": "gpt-4-0613",
  "input_price_per_1m": "30.00000000",
  "output_price_per_1m": "60.00000000",
  "cached_input_price_per_1m": "15.00000000",
  "currency": "USD",
  "updated_at": "2026-09-08T02:10:00Z"
}
```

### 2.3 删除单价

`DELETE /admin/pricing`

```json
{ "channel_id": 2, "model_name": "gpt-4" }
```

删除后该渠道该模型调用按 0 价计费（费用入账为 0，token 仍统计）。建议改用「设为 0 价」以保留记录可追溯。

---

## 3. 错误码补充

| code | 含义 |
| --- | --- |
| `30000` | 定价记录不存在（删除/更新时） |
| `30001` | 价格必须 ≥ 0 且位数合法 |
| `30002` | `(channel_id, model_name)` 对应的模型映射不存在，先发布模型 |
| `30003` | currency 仅支持已配置货币（默认 USD） |
