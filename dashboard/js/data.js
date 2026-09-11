/* ============================================================
   模拟数据 —— 字段结构对齐 /admin 管理端接口文档
   （stats/overview · usage_logs · channels · user_daily_stats ·
     users/balance · rate-limit rules）
   ============================================================ */

/** GET /admin/stats/overview —— 总览卡片 */
const OVERVIEW = {
  request_count: 128431,
  success_count: 126758,
  error_count: 1673,
  total_tokens: 12482931,      // 约 12.5M
  total_cost: '520.36842100',  // 美元
  active_user_count: 42,
};

/** GET /admin/stats/daily —— 近 14 天用户日汇总 */
const DAILY_STATS = [
  { date: '08-29', label: '8/29', requests: 6420,  cost: 24.10 },
  { date: '08-30', label: '8/30', requests: 7105,  cost: 27.85 },
  { date: '08-31', label: '8/31', requests: 6880,  cost: 25.60 },
  { date: '09-01', label: '9/01', requests: 8240,  cost: 32.40 },
  { date: '09-02', label: '9/02', requests: 9012,  cost: 36.75 },
  { date: '09-03', label: '9/03', requests: 8790,  cost: 33.20 },
  { date: '09-04', label: '9/04', requests: 9530,  cost: 38.90 },
  { date: '09-05', label: '9/05', requests: 10842, cost: 44.30 },
  { date: '09-06', label: '9/06', requests: 11206, cost: 46.80 },
  { date: '09-07', label: '9/07', requests: 10120, cost: 41.25 },
  { date: '09-08', label: '9/08', requests: 11873, cost: 49.10 },
  { date: '09-09', label: '9/09', requests: 12540, cost: 52.65 },
  { date: '09-10', label: '9/10', requests: 13008, cost: 55.30 },
  { date: '09-11', label: '9/11', requests: 13965, cost: 60.15 },
];

/** GET /admin/channels —— 上游渠道（含路由/健康元数据） */
const CHANNELS = [
  { id: 1, name: 'Azure OpenAI 生产', base_url: 'https://azure-east.openai.azure.com', status: 1, weight: 100, priority: 10, health: 99.8, success_rate: 99.2, rpm: 812, models: ['gpt-4o', 'gpt-4o-mini'], cost_24h: 231.40, circuit: 'closed' },
  { id: 2, name: 'Anthropic 官方',    base_url: 'https://api.anthropic.com',          status: 1, weight: 80,  priority: 10, health: 99.4, success_rate: 98.7, rpm: 455, models: ['claude-3-5-sonnet', 'claude-3-haiku'], cost_24h: 168.75, circuit: 'closed' },
  { id: 3, name: 'OpenAI 直连',       base_url: 'https://api.openai.com',             status: 1, weight: 60,  priority: 5,  health: 97.1, success_rate: 96.3, rpm: 388, models: ['gpt-4o', 'gpt-4o-mini', 'o1-mini'], cost_24h: 92.10, circuit: 'closed' },
  { id: 4, name: '第三方中转 A',      base_url: 'https://relay-a.example.com',        status: 1, weight: 40,  priority: 0,  health: 91.6, success_rate: 93.8, rpm: 120, models: ['gpt-4o', 'deepseek-chat'], cost_24h: 28.02, circuit: 'half-open' },
  { id: 5, name: '备用中转 B',        base_url: 'https://relay-b.example.com',        status: 0, weight: 20,  priority: -5, health: 62.3, success_rate: 84.1, rpm: 0,   models: ['gpt-3.5-turbo'], cost_24h: 0.11, circuit: 'open' },
];

/** 对外模型用量分布（基于 usage_logs 聚合，万 tokens） */
const MODEL_DIST = [
  { name: 'gpt-4o',          value: 482 },
  { name: 'claude-3-5-sonnet', value: 301 },
  { name: 'gpt-4o-mini',     value: 186 },
  { name: 'deepseek-chat',   value: 97 },
  { name: 'o1-mini',         value: 43 },
];

/** GET /admin/usage-logs —— 最近请求明细 */
const RECENT_LOGS = [
  { request_id: '01JXYZ4A8F', user: 'AlphaLab',  model: 'gpt-4o',            channel: 'Azure OpenAI 生产', input_tokens: 1240, output_tokens: 862,  total_cost: '0.0824', duration_ms: 8120, ttft_ms: 620,  status: 'success', created_at: '09:22:41' },
  { request_id: '01JXYZ39C2', user: 'NovaCRM',   model: 'claude-3-5-sonnet', channel: 'Anthropic 官方',    input_tokens: 3310, output_tokens: 2415, total_cost: '0.1432', duration_ms: 15400, ttft_ms: 890, status: 'success', created_at: '09:22:18' },
  { request_id: '01JXYZ2E77', user: 'AlphaLab',  model: 'gpt-4o-mini',       channel: 'Azure OpenAI 生产', input_tokens: 486,  output_tokens: 305,  total_cost: '0.0041', duration_ms: 2380, ttft_ms: 310,  status: 'success', created_at: '09:21:57' },
  { request_id: '01JXYZ1B0D', user: 'ByteDocs',  model: 'deepseek-chat',     channel: '第三方中转 A',      input_tokens: 2050, output_tokens: 1780, total_cost: '0.0118', duration_ms: 9900, ttft_ms: 1450, status: 'success', created_at: '09:21:30' },
  { request_id: '01JXYZ09F3', user: 'NovaCRM',   model: 'gpt-4o',            channel: 'OpenAI 直连',       input_tokens: 812,  output_tokens: 0,    total_cost: '0.0000', duration_ms: 1820, ttft_ms: 0,    status: 'error',   created_at: '09:20:52' },
  { request_id: '01JXXYZZ81', user: 'QuantEdge', model: 'o1-mini',           channel: 'OpenAI 直连',       input_tokens: 5230, output_tokens: 3120, total_cost: '0.2095', duration_ms: 42100, ttft_ms: 2100, status: 'success', created_at: '09:20:11' },
  { request_id: '01JXXYZE19', user: 'ByteDocs',  model: 'gpt-4o',            channel: 'Azure OpenAI 生产', input_tokens: 960,  output_tokens: 714,  total_cost: '0.0618', duration_ms: 7040, ttft_ms: 540,  status: 'success', created_at: '09:19:44' },
];

/** GET /admin/users —— 用户余额 Top（美元，字符串金额对齐文档） */
const TOP_USERS = [
  { name: 'AlphaLab',  group: 'vip',  available_balance: '842.56',  frozen_balance: '12.30',  qpd: 48200, status: 'active' },
  { name: 'NovaCRM',   group: 'vip',  available_balance: '617.20',  frozen_balance: '8.75',   qpd: 36410, status: 'active' },
  { name: 'QuantEdge', group: 'std',  available_balance: '233.08',  frozen_balance: '21.60',  qpd: 19880, status: 'active' },
  { name: 'ByteDocs',  group: 'std',  available_balance: '58.42',   frozen_balance: '3.10',   qpd: 9210,  status: 'active' },
  { name: 'TestUser',  group: 'free', available_balance: '2.31',    frozen_balance: '0.00',   qpd: 640,   status: 'suspended' },
];

/** 限流规则（rate_limit_rules）概览 */
const RATE_LIMITS = [
  { scope: 'global', target: '全局限流',  rpm: 2000, tpm: 800_000, tpd: 200_000_000, conc: 300, usage: 68 },
  { scope: 'model',  target: 'gpt-4o',    rpm: 600,  tpm: 240_000, tpd: 60_000_000,  conc: 120, usage: 91 },
  { scope: 'user',   target: 'AlphaLab',  rpm: 300,  tpm: 120_000, tpd: 30_000_000,  conc: 60,  usage: 74 },
  { scope: 'user',   target: 'NovaCRM',   rpm: 240,  tpm: 96_000,  tpd: 24_000_000,  conc: 50,  usage: 57 },
  { scope: 'channel',target: 'OpenAI 直连', rpm: 480, tpm: 192_000, tpd: 48_000_000,  conc: 100, usage: 82 },
];

/** 通知 */
const NOTIFICATIONS = [
  { tone: 'warning', text: '渠道「第三方中转 A」5xx 比例升高，熔断器进入 half-open' },
  { tone: 'success', text: '昨日账单结算完成：$52.65，已同步 balance_transactions' },
  { tone: 'cyan',    text: '新渠道「OpenAI 直连」已启用并纳入路由（优先级 5）' },
];

/** API 文档快捷入口 */
const API_DOCS = [
  { title: 'Chat Completions', desc: 'OpenAI 兼容下游接口', href: '../docs/01-下游接口/chat-completions.md', icon: 'zap' },
  { title: '模型列表', desc: 'GET /v1/models', href: '../docs/01-下游接口/models.md', icon: 'models' },
  { title: '渠道管理', desc: 'GET /admin/channels', href: '../docs/02-管理端接口/channels.md', icon: 'channels' },
  { title: '统计与账单', desc: 'usage_logs / daily stats', href: '../docs/02-管理端接口/stats-billing.md', icon: 'chart' },
];

/** 路由策略与队列概览 */
const ROUTING_OVERVIEW = [
  { label: '模型路由命中率', value: 99.4, right: '99.4%' },
  { label: '低余额渠道剔除', value: 18, right: '18%' },
  { label: '熔断探测通过', value: 72, right: '72%' },
];

const ERROR_MIX = [
  { name: 'rate_limit_exceeded', count: 842, tone: 'warning' },
  { name: 'upstream_timeout', count: 391, tone: 'error' },
  { name: 'insufficient_balance', count: 266, tone: 'neutral' },
  { name: 'channel_unavailable', count: 174, tone: 'cyan' },
];
