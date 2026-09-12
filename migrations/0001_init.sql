-- ============================================================================
-- MyApi 数据库表结构（PostgreSQL 12+）
-- ----------------------------------------------------------------------------
-- 与 internal/model/*.go 的 GORM 模型保持一致，作为生产环境手动建表的事实来源。
-- 开发环境也可将 configs/config.yaml 的 database.auto_migrate 设为 true，
-- 由 GORM AutoMigrate 自动建表。
--
-- 执行：psql -U postgres -d myapi -f migrations/0001_init.sql
--
-- 说明：
--   * updated_at 由应用（GORM）维护，这里不建触发器。
--   * usage_logs.total_tokens 为生成列，应用只读、不写入。
--   * 金额/单价统一使用 NUMERIC，避免浮点误差。
-- ============================================================================

-- ---------------------------------------------------------------------------
-- 1. 上游渠道
-- ---------------------------------------------------------------------------
CREATE TABLE channels (
    id                 BIGSERIAL    PRIMARY KEY,
    name               VARCHAR(100) NOT NULL,                    -- 渠道名称
    base_url           VARCHAR(255) NOT NULL,                    -- 上游 OpenAI 兼容地址
    api_key            VARCHAR(500) NOT NULL,                    -- 上游密钥密文（enc:v1: 前缀）
    auth_type          VARCHAR(20)  DEFAULT 'bearer',            -- 认证方式，目前仅 bearer
    extra_config       JSONB,                                    -- 渠道扩展配置
    status             SMALLINT     NOT NULL DEFAULT 1,          -- 1 启用 / 0 停用
    weight             INTEGER      DEFAULT 100,                 -- 同优先级下的负载均衡权重
    priority           INTEGER      DEFAULT 0,                   -- 路由优先级，数值越大越优先
    balance            NUMERIC(14,8),                            -- 本地余额，NULL 表示不限
    balance_updated_at TIMESTAMPTZ,                              -- 余额最后更新时间
    created_at         TIMESTAMPTZ  DEFAULT CURRENT_TIMESTAMP,
    updated_at         TIMESTAMPTZ  DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_channels_active_priority ON channels (priority DESC) WHERE status = 1;

-- ---------------------------------------------------------------------------
-- 2. 渠道模型映射（对外模型名 -> 上游真实模型名）
-- ---------------------------------------------------------------------------
CREATE TABLE channel_models (
    id             BIGSERIAL    PRIMARY KEY,
    channel_id     BIGINT       NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    model_name     VARCHAR(100) NOT NULL,                        -- 网关对外模型名
    upstream_model VARCHAR(100) NOT NULL,                        -- 上游真实模型名
    enabled        BOOLEAN      DEFAULT TRUE,
    created_at     TIMESTAMPTZ  DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMPTZ  DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_channel_model UNIQUE (channel_id, model_name)
);

CREATE INDEX idx_model_name ON channel_models (model_name);

-- ---------------------------------------------------------------------------
-- 3. 渠道 × 模型 定价（美元 / 1M tokens）
-- ---------------------------------------------------------------------------
CREATE TABLE model_pricing (
    id                        BIGSERIAL     PRIMARY KEY,
    channel_id                BIGINT        NOT NULL,
    model_name                VARCHAR(100)  NOT NULL,
    input_price_per_1m        NUMERIC(12,8) NOT NULL DEFAULT 0,
    output_price_per_1m       NUMERIC(12,8) NOT NULL DEFAULT 0,
    cached_input_price_per_1m NUMERIC(12,8),                     -- NULL 时回退为输入价
    currency                  VARCHAR(3)    NOT NULL DEFAULT 'USD',
    created_at                TIMESTAMPTZ   DEFAULT CURRENT_TIMESTAMP,
    updated_at                TIMESTAMPTZ   DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_channel_model_pricing UNIQUE (channel_id, model_name),
    CONSTRAINT fk_pricing_channel_model FOREIGN KEY (channel_id, model_name)
        REFERENCES channel_models(channel_id, model_name) ON DELETE CASCADE,
    CONSTRAINT check_pricing_positive CHECK (input_price_per_1m >= 0 AND output_price_per_1m >= 0)
);

CREATE INDEX idx_pricing ON model_pricing (channel_id, model_name);

-- ---------------------------------------------------------------------------
-- 4. 下游用户（计费主体 / 租户）
-- ---------------------------------------------------------------------------
CREATE TABLE users (
    id            BIGSERIAL    PRIMARY KEY,
    password_hash TEXT         NOT NULL,                         -- bcrypt/Argon2 哈希
    user_group    VARCHAR(20)  NOT NULL DEFAULT 'default',       -- default / vip / enterprise
    status        VARCHAR(20)  NOT NULL DEFAULT 'active',        -- active / suspended / deleted
    nickname      VARCHAR(100),
    last_login_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_users_status ON users (status);

-- ---------------------------------------------------------------------------
-- 5. 用户余额账户
-- ---------------------------------------------------------------------------
CREATE TABLE user_balances (
    user_id           BIGINT        PRIMARY KEY,
    available_balance NUMERIC(12,6) NOT NULL DEFAULT 0,          -- 可用余额
    frozen_balance    NUMERIC(12,6) NOT NULL DEFAULT 0,          -- 冻结余额
    version           BIGINT        NOT NULL DEFAULT 1,          -- 乐观锁版本号
    updated_at        TIMESTAMPTZ   DEFAULT CURRENT_TIMESTAMP,
    created_at        TIMESTAMPTZ   DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT check_balance_non_negative CHECK (available_balance >= 0)
);

-- ---------------------------------------------------------------------------
-- 6. 资金流水（不可变）
-- ---------------------------------------------------------------------------
CREATE TABLE balance_transactions (
    id                BIGSERIAL     PRIMARY KEY,
    user_id           BIGINT        NOT NULL,
    amount            NUMERIC(12,6) NOT NULL,                    -- 正=充值/退款，负=消费/扣减
    balance_before    NUMERIC(12,6) NOT NULL,
    balance_after     NUMERIC(12,6) NOT NULL,
    tx_type           VARCHAR(20)   NOT NULL,                    -- recharge/consume/refund/freeze/unfreeze
    related_request   VARCHAR(64),                               -- 关联请求 ID（接口字段：related_request_id）
    related_order_id  VARCHAR(64),                               -- 关联充值订单号
    description       TEXT,
    created_at        TIMESTAMPTZ   DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT check_amount_non_zero CHECK (amount <> 0)
);

CREATE INDEX idx_tx_user_created ON balance_transactions (user_id, created_at DESC);

-- ---------------------------------------------------------------------------
-- 7. 下游网关 Key（只存哈希）
-- ---------------------------------------------------------------------------
CREATE TABLE client_api_keys (
    id                   BIGSERIAL    PRIMARY KEY,
    user_id              BIGINT       NOT NULL,
    key_name             VARCHAR(100) NOT NULL DEFAULT 'default',
    key_prefix           VARCHAR(10)  NOT NULL,                  -- 如 sk-
    key_hash             TEXT         NOT NULL,                  -- SHA-256 哈希，绝不存明文
    permissions          JSONB        DEFAULT '{"models": ["*"]}'::jsonb,
    rate_limit_overrides JSONB        DEFAULT '{}'::jsonb,
    expires_at           TIMESTAMPTZ,                            -- NULL 表示永不过期
    is_active            BOOLEAN      NOT NULL DEFAULT TRUE,
    last_used_at         TIMESTAMPTZ,                            -- 保留字段（活跃度由 usage_logs 派生）
    created_at           TIMESTAMPTZ  DEFAULT CURRENT_TIMESTAMP,
    updated_at           TIMESTAMPTZ  DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_user_keyname UNIQUE (user_id, key_name)
);

CREATE INDEX idx_api_keys_auth ON client_api_keys (key_prefix, key_hash)
    WHERE is_active = TRUE AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP);

-- ---------------------------------------------------------------------------
-- 8. 限流规则
-- ---------------------------------------------------------------------------
CREATE TABLE rate_limit_rules (
    id             BIGSERIAL    PRIMARY KEY,
    rule_name      VARCHAR(100) NOT NULL,
    target_type    VARCHAR(20)  NOT NULL,                        -- global/user/api_key/model/channel
    target_value   TEXT         NOT NULL DEFAULT '*',            -- * 通配或具体值
    metric         VARCHAR(20)  NOT NULL,                        -- rpm/tpm/rpd/tpd/concurrency
    limit_value    INTEGER      NOT NULL,
    window_seconds INTEGER      NOT NULL,
    action         VARCHAR(20)  DEFAULT 'reject',                -- reject / queue
    priority       INTEGER      DEFAULT 0,                       -- 数值越小优先级越高
    enabled        BOOLEAN      DEFAULT TRUE,
    description    TEXT,
    extras         JSONB        DEFAULT '{}'::jsonb,             -- 如 queue_timeout_seconds
    created_at     TIMESTAMPTZ  DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMPTZ  DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT check_target_type CHECK (target_type IN ('global', 'user', 'api_key', 'model', 'channel')),
    CONSTRAINT check_metric CHECK (metric IN ('rpm', 'tpm', 'rpd', 'tpd', 'concurrency'))
);

CREATE INDEX idx_rate_limit_lookup ON rate_limit_rules (target_type, target_value, metric, priority)
    WHERE enabled = TRUE;
CREATE INDEX idx_enabled_priority ON rate_limit_rules (priority ASC) WHERE enabled = TRUE;

-- ---------------------------------------------------------------------------
-- 9. 用量明细（逐请求）
-- ---------------------------------------------------------------------------
CREATE TABLE usage_logs (
    id                      BIGSERIAL     PRIMARY KEY,
    request_id              VARCHAR(64)   NOT NULL UNIQUE,       -- 网关请求 ID
    user_id                 BIGINT        NOT NULL,
    api_key_id              BIGINT        NOT NULL,
    channel_id              BIGINT        NOT NULL,
    model                   VARCHAR(100)  NOT NULL,              -- 对外模型名
    upstream_model          VARCHAR(100),                        -- 上游真实模型名
    input_tokens            INTEGER       NOT NULL DEFAULT 0,
    output_tokens           INTEGER       NOT NULL DEFAULT 0,
    cached_input_tokens     INTEGER       DEFAULT 0,
    total_tokens            INTEGER       GENERATED ALWAYS AS (input_tokens + output_tokens) STORED,
    unit_price_input_per_1m  NUMERIC(12,8) NOT NULL,             -- 费用快照
    unit_price_output_per_1m NUMERIC(12,8) NOT NULL,
    total_cost              NUMERIC(12,8) NOT NULL,
    duration_ms             INTEGER       NOT NULL,
    ttft_ms                 INTEGER,
    status                  VARCHAR(20)   NOT NULL DEFAULT 'success',  -- success / error
    error_code              VARCHAR(50),
    client_ip               INET,
    extra                   JSONB         DEFAULT '{}'::jsonb,
    created_at              TIMESTAMPTZ   NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_usage_user_created    ON usage_logs (user_id, created_at DESC);
CREATE INDEX idx_usage_channel_created ON usage_logs (channel_id, created_at DESC);
CREATE INDEX idx_usage_key_created     ON usage_logs (api_key_id, created_at DESC);
CREATE INDEX idx_usage_status_created  ON usage_logs (status, created_at DESC);
CREATE INDEX idx_usage_user_model_cost ON usage_logs (user_id, created_at DESC)
    INCLUDE (model, input_tokens, output_tokens, total_cost);

-- ---------------------------------------------------------------------------
-- 10. 用户日汇总（用户 × 日期）
-- ---------------------------------------------------------------------------
CREATE TABLE user_daily_stats (
    user_id                   BIGINT        NOT NULL,
    stat_date                 DATE          NOT NULL,
    total_input_tokens        BIGINT        DEFAULT 0,
    total_output_tokens       BIGINT        DEFAULT 0,
    total_cached_input_tokens BIGINT        DEFAULT 0,
    total_tokens              BIGINT        DEFAULT 0,
    total_cost                NUMERIC(12,8) DEFAULT 0,
    request_count             INTEGER       DEFAULT 0,
    success_count             INTEGER       DEFAULT 0,
    error_count               INTEGER       DEFAULT 0,
    last_processed_id         BIGINT        DEFAULT 0,           -- 已处理到的 usage_logs.id
    updated_at                TIMESTAMPTZ   DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, stat_date)
);
