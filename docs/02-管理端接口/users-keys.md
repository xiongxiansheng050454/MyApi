# 管理端接口 — 用户 · 网关 Key · 余额

管理端所有接口路径前缀 `/admin`，**不设认证**（部署于内网/受控网络），请求/响应格式见 [附录](../03-附录.md)。

- 金额一律使用**字符串**表示（单位：美元），避免浮点精度丢失。
- 分页参数：`page`（默认 1）、`page_size`（默认 20，最大 100）。

---

## 1. 用户管理

### 1.1 创建用户

`POST /admin/users`

```json
{
  "nickname": "某公司-A组",
  "user_group": "default",
  "status": "active",
  "password": "可选，登录用；不传则由网关生成随机密码"
}
```

| 字段 | 类型 | 必填 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `nickname` | string | 否 | null | 展示名 |
| `user_group` | string | 否 | `default` | `default / vip / enterprise`，用于路由与限流 |
| `status` | string | 否 | `active` | `active / suspended / deleted` |
| `password` | string | 否 | 随机 | 网关存 bcrypt 哈希；返回明文一次 |

响应 `data`：

```json
{
  "id": 1001,
  "nickname": "某公司-A组",
  "user_group": "default",
  "status": "active",
  "password_plaintext": "仅首次返回",
  "created_at": "2026-09-08T02:00:00Z"
}
```

### 1.2 用户列表

`GET /admin/users?page=1&page_size=20&status=active&user_group=vip&keyword=某公司`

| 查询参数 | 类型 | 说明 |
| --- | --- | --- |
| `status` | string | 过滤状态 |
| `user_group` | string | 过滤分组 |
| `keyword` | string | 按 id / nickname 模糊匹配 |
| `page` / `page_size` | int | 分页 |

响应 `data`：

```json
{
  "list": [
    {
      "id": 1001,
      "nickname": "某公司-A组",
      "user_group": "default",
      "status": "active",
      "last_login_at": null,
      "created_at": "2026-09-08T02:00:00Z",
      "balance": { "available_balance": "100.000000", "frozen_balance": "0.000000" }
    }
  ],
  "total": 3,
  "page": 1,
  "page_size": 20
}
```

### 1.3 用户详情

`GET /admin/users/{userId}`

返回用户基本信息 + `balance`（`user_balances` 实时值）。

### 1.4 更新用户

`PUT /admin/users/{userId}`

| 字段 | 说明 |
| --- | --- |
| `nickname` | 更新展示名 |
| `user_group` | 更新分组 |
| `password` | 重置密码（返回明文一次） |

`status` 变更请使用 1.5 独立接口。

### 1.5 变更用户状态

`PUT /admin/users/{userId}/status`

```json
{ "status": "suspended" }
```

| 状态 | 效果 |
| --- | --- |
| `suspended` | 该用户下所有 Key 立即拒绝调用（`403 account_suspended`） |
| `deleted` | 逻辑删除，拒绝调用；历史账单保留 |
| `active` | 恢复 |

> 不建议物理删除用户；如需彻底删除需确认无关联流水（返回冲突错误码）。

---

## 2. 余额与充值

### 2.1 查询余额

`GET /admin/users/{userId}/balance`

```json
{
  "available_balance": "95.000000",
  "frozen_balance": "5.000000",
  "updated_at": "2026-09-08T05:00:00Z"
}
```

### 2.2 充值

`POST /admin/users/{userId}/recharge`

```json
{
  "amount": "50.000000",
  "related_order_id": "PAY-20260908-0001",
  "description": "线下转账充值"
}
```

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `amount` | string | 是 | > 0，最多 6 位小数，美元 |
| `related_order_id` | string | 否 | 幂等依据；网关建议对同 order 去重 |
| `description` | string | 否 | 备注 |

响应：落一笔 `balance_transactions`（`tx_type=recharge`，含 `balance_before/after` 快照）。

```json
{
  "tx_id": 900001,
  "balance_before": "95.000000",
  "balance_after": "145.000000"
}
```

### 2.3 资金流水

`GET /admin/users/{userId}/balance-transactions?page=1&page_size=20&tx_type=consume&start_time=2026-09-01T00:00:00Z&end_time=2026-09-08T23:59:59Z`

| 查询参数 | 说明 |
| --- | --- |
| `tx_type` | `recharge / consume / refund / freeze / unfreeze` |
| `start_time` / `end_time` | RFC3339，按 `created_at` 过滤 |
| `page` / `page_size` | 分页 |

`list` 项字段：

```json
{
  "id": 900002,
  "tx_type": "consume",
  "amount": "-0.001234",
  "balance_before": "5.000000",
  "balance_after": "4.998766",
  "related_request_id": "01JXYZ...",
  "related_order_id": null,
  "description": null,
  "created_at": "2026-09-08T05:00:01Z"
}
```

---

## 3. 网关 Key 管理

Key 的明文结构为 `<key_prefix><密钥>`（默认前缀 `sk-`，如 `sk-aB3...`）。数据库只存 `key_prefix` 与完整 Key 的哈希，**明文仅在下发/重置时返回一次，不可再次查询**。

### 3.1 创建 Key

`POST /admin/users/{userId}/keys`

```json
{
  "key_name": "生产环境",
  "prefix": "sk-",
  "permissions": { "models": ["*"] },
  "rate_limit_overrides": { "rpm": 600, "tpm": 120000 },
  "expires_at": "2027-09-08T00:00:00Z",
  "is_active": true
}
```

| 字段 | 类型 | 必填 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `key_name` | string | 否 | `default` | 同一用户下唯一 |
| `prefix` | string | 否 | `sk-` | 1~10 字符（`[A-Za-z0-9-_]`），用于识别来源 |
| `permissions.models` | string[] | 否 | `["*"]` | 允许调用的对外模型名白名单；`*` 全部 |
| `rate_limit_overrides` | object | 否 | `{}` | 单 Key 限速覆盖，见 [结算与限速语义](../01-下游接口/结算与限速语义.md) |
| `expires_at` | string | 否 | null | RFC3339；空 = 永不过期 |
| `is_active` | bool | 否 | true | 停用后立即拒绝 |

响应 `data`（**full_key 仅此一次**）：

```json
{
  "id": 5501,
  "key_name": "生产环境",
  "prefix": "sk-",
  "full_key": "sk-aB3dEf9...",   
  "permissions": { "models": ["*"] },
  "rate_limit_overrides": { "rpm": 600, "tpm": 120000 },
  "expires_at": "2027-09-08T00:00:00Z",
  "is_active": true,
  "created_at": "2026-09-08T02:00:00Z"
}
```

> 下游使用 `Authorization: Bearer <full_key>`。请务必保存，网关无法再次展示。

### 3.2 Key 列表

`GET /admin/users/{userId}/keys?page=1&page_size=20&is_active=true`

响应不含 `full_key`；包含 `last_used_at`。也支持全局检索：`GET /admin/keys?user_id=&is_active=&key_name=`。

### 3.3 更新 Key

`PUT /admin/users/{userId}/keys/{keyId}`

可更新 `key_name`、`permissions`、`rate_limit_overrides`、`expires_at`、`is_active`。**不改变 Key 明文**。

### 3.4 重置 Key（轮换密钥）

`POST /admin/users/{userId}/keys/{keyId}/reset`

重新生成密钥（`full_key` 变化），返回明文一次；旧 Key 立即失效。用于疑似泄露场景。`key_id` 不变，历史 `usage_logs.api_key_id` 仍可追溯。

### 3.5 删除 Key

`DELETE /admin/users/{userId}/keys/{keyId}`

立即失效并删除记录。若该 Key 已有历史用量，可改为「3.3 停用」以保留审计关联。

---

## 4. 错误码补充

除 [附录通用错误码](../03-附录.md) 外，本模块可能返回：

| code | 含义 |
| --- | --- |
| `10100` | 用户不存在 |
| `10101` | 用户状态不允许该操作（如对 deleted 用户充值） |
| `10102` | 充值金额必须 > 0 |
| `10200` | Key 不存在 |
| `10201` | 同一用户下 key_name 已存在 |
| `10202` | prefix 非法 |
| `10203` | 用户已停用，禁止创建/启用 Key |
