# MyApi — LLM 统一接入网关

用一个地址、一把 Key，统一调用多家大模型；在一个控制台里管理渠道、额度、费用和调用记录。

MyApi 位于你的业务和上游大模型之间。业务方只对接一个 OpenAI 兼容入口，网关负责把请求转发到合适的上游，并完成鉴权、限速、限额、计费与统计。你不需要再为每一家上游分别改代码、换密钥。

---

## 它能帮你做什么

- **一个入口接所有模型**：下游继续用 OpenAI SDK，只改 `base_url` 和 Key。
- **多上游自由切换**：同一模型可配置多个上游渠道，按权重分流、按优先级降级。
- **上游故障自动兜底**：某个渠道连续失败会被熔断，请求自动切到健康渠道。
- **按人发 Key、控额度**：给每个用户/项目签发独立网关 Key，单独限速、限额。
- **用量与费用看得见**：按用户、Key、模型、渠道查看 token、次数和费用。
- **密钥不外泄**：下游只看到网关 Key，上游真实密钥加密保存。

---

## 快速开始

### 前置条件

- 已安装 Docker 与 Docker Compose（推荐），或
- 本地已安装 Go 1.26+、PostgreSQL、Redis

### 方式一：Docker Compose 一键启动（推荐）

```bash
docker compose up --build
```

启动后：

| 入口 | 地址 | 用途 |
| --- | --- | --- |
| 管理控制台 | http://localhost:8080/dashboard/ | 运营方管理界面 |
| 接口文档 | http://localhost:8080/docs/ | 在线查看接口说明 |
| 下游 API | http://localhost:8080/v1 | 业务方调用入口 |
| 健康检查 | http://localhost:8080/healthz | 探活 |

> 数据（PostgreSQL / Redis / 加密密钥）通过 Docker 卷持久化，重启不丢。

### 方式二：本地开发运行

1. 启动本地 PostgreSQL 与 Redis，并建库 `myapi`。
2. 复制并按需修改配置：

```bash
copy configs\config.example.yaml configs\config.yaml
```

3. 运行：

```bash
go run ./cmd/server -config configs/config.yaml
```

> 首次启动会自动生成上游密钥加密主密钥文件 `data/apikey_enc.key`，请妥善保管；也可通过环境变量 `MYAPI_APIKEY_ENC_KEY` 指定。

---

## 上手：接入第一个模型（5 步）

打开管理控制台 http://localhost:8080/dashboard/ ，按顺序操作：

1. **添加上游渠道**：填写渠道名称、`base_url`（上游 OpenAI 兼容地址）、上游 `api_key`。可点“测试”验证连通性。
2. **配置模型映射**：为该渠道添加「对外模型名 → 上游真实模型名」，例如 `gpt-4o → gpt-4o-2024-08-06`。
3. **设置单价**：为该「渠道 × 模型」填写输入/输出单价（美元 / 100 万 tokens）。
4. **创建用户并签发 Key**：新建下游用户，创建网关 Key。**明文 Key 只在创建/重置时显示一次**，请立即复制保存。
5. **下游开始调用**：业务方把 OpenAI SDK 的 `base_url` 指向网关，使用刚签发的 Key。

> 同一个对外模型名可以映射到多个渠道，网关会自动做负载均衡与故障切换。

---

## 下游怎么调用

### curl

```bash
curl http://localhost:8080/v1/chat/completions ^
  -H "Authorization: Bearer sk-你的网关Key" ^
  -H "Content-Type: application/json" ^
  -d "{\"model\":\"gpt-4o\",\"messages\":[{\"role\":\"user\",\"content\":\"你好\"}]}"
```

### OpenAI Python SDK

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8080/v1",
    api_key="sk-你的网关Key",
)

resp = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "你好"}],
)
print(resp.choices[0].message.content)
```

流式只需加 `stream=True`，用法与 OpenAI 完全一致。

---

## 日常运营

- **限速与限额**：在控制台“限流规则”里按 global / 用户 / Key / 模型维度配置 rpm、tpm、rpd、tpd、并发，命中后拒绝或排队。
- **用户余额**：在“用户”里为账户充值；余额不足的请求会被拒绝。调用费用按单价自动结算。
- **渠道健康**：渠道连续失败会自动熔断并切换；控制台可查看状态、手动测试连通性。
- **查看用量**：在“用量/统计”里按用户、Key、模型、渠道筛选 token、次数与费用明细。
- **上游模型**：新增渠道时可直接拉取上游模型列表，批量建立映射。

---

## 配置

配置集中在 `configs/config.yaml`，常见项：

- 监听端口、控制台/文档目录
- PostgreSQL、Redis 连接
- 路由（粘性、重试、降级、余额低水位）
- 计费（预冻结、默认输出预算、分词编码）
- 限流（规则缓存、token 估算系数）
- 统计（刷入间隔、时区）
- 日志级别与格式

所有项均可用环境变量 `MYAPI_*` 覆盖，详见 `configs/config.example.yaml` 内注释。

---

## 文档

- 接口文档：`docs/README.md`（运行后在 http://localhost:8080/docs/ 在线阅读）
  - 下游接口：Chat Completions、模型列表、结算与限速语义
  - 管理端接口：用户与 Key、渠道、定价、限流、统计
  - 附录：错误码、枚举、分页与数据类型约定
- 架构与开发约定：`AGENTS.md`

---

## 常见问题

**Q：管理端需要登录吗？**
A：当前 `/admin` 不设鉴权，依赖内网部署，请勿直接暴露公网。

**Q：下游的 Key 和上游的密钥是同一个吗？**
A：不是。下游只持有“网关 Key”；上游真实密钥由网关加密保存，仅转发瞬间解密使用。

**Q：忘记网关 Key 明文了怎么办？**
A：数据库只存哈希，无法找回。在控制台对对应 Key 执行“重置”，会生成新的明文并展示一次。

**Q：可以同时接多个同款模型的不同上游吗？**
A：可以。给同一对外模型名配置多个渠道，再设置各自权重即可分流。

**Q：为什么请求返回“余额不足”？**
A：网关会在转发前按预估费用预冻结额度。请为用户充值，或调整该 Key/用户的限额。
