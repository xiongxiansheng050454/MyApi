/* ============================================================
   仪表盘装配 · 视图路由 · 文档阅读器
   ============================================================ */

/* ---------- 侧边栏导航 ---------- */
const NAV_ITEMS = [
  { group: '总览' },
  { key: 'dashboard', name: '仪表盘',   icon: 'dashboard' },
  { key: 'usage',     name: '用量统计', icon: 'chart' },
  { key: 'logs',      name: '请求日志', icon: 'logs', badge: '1.7k' },
  { key: 'docs',      name: 'API 文档', icon: 'book' },
  { group: '资源管理' },
  { key: 'channels',  name: '渠道管理', icon: 'channels' },
  { key: 'models',    name: '模型路由', icon: 'models' },
  { key: 'users',     name: '用户与 Key', icon: 'users' },
  { group: '运营' },
  { key: 'limits',    name: '限流规则', icon: 'gauge' },
  { key: 'pricing',   name: '计费定价', icon: 'wallet' },
  { key: 'settings',  name: '系统设置', icon: 'settings' },
];

const DOC_LIST = [
  { title: '文档总览',            href: '../docs/README.md' },
  { title: 'Chat Completions',   href: '../docs/01-下游接口/chat-completions.md' },
  { title: 'Models',             href: '../docs/01-下游接口/models.md' },
  { title: '结算与限速语义',       href: '../docs/01-下游接口/结算与限速语义.md' },
  { title: '渠道管理',            href: '../docs/02-管理端接口/channels.md' },
  { title: '用户与 Key',          href: '../docs/02-管理端接口/users-keys.md' },
  { title: '计费定价',            href: '../docs/02-管理端接口/pricing.md' },
  { title: '限流规则',            href: '../docs/02-管理端接口/rate-limits.md' },
  { title: '统计与账单',          href: '../docs/02-管理端接口/stats-billing.md' },
  { title: '附录',               href: '../docs/03-附录.md' },
];

let LAST_UPDATED = '';

/* ============================================================
   通用片段
   ============================================================ */
function logsTableHTML(logs) {
  if (!logs || !logs.length) return `<div class="py-10 text-center text-xs text-zinc-500">暂无请求日志</div>`;
  return `
    <div class="overflow-x-auto">
      <table class="w-full text-left text-xs">
        <thead>
          <tr class="border-b border-zinc-800 text-[11px] uppercase tracking-wide text-zinc-500">
            <th class="pb-2.5 pr-4 font-medium">时间</th>
            <th class="pb-2.5 pr-4 font-medium">用户 / 模型</th>
            <th class="pb-2.5 pr-4 font-medium">路由渠道</th>
            <th class="pb-2.5 pr-4 font-medium text-right">Tokens</th>
            <th class="pb-2.5 pr-4 font-medium text-right">TTFT</th>
            <th class="pb-2.5 pr-4 font-medium text-right">耗时</th>
            <th class="pb-2.5 pr-4 font-medium text-right">费用</th>
            <th class="pb-2.5 font-medium text-right">状态</th>
          </tr>
        </thead>
        <tbody>
          ${logs.map((l) => `
            <tr class="row-hover border-b border-zinc-800/50">
              <td class="py-2.5 pr-4 font-mono text-zinc-500">${l.created_at}</td>
              <td class="py-2.5 pr-4">
                <div class="font-semibold">${l.user}</div>
                <div class="font-mono text-[10px] text-zinc-500">${l.model}</div>
              </td>
              <td class="py-2.5 pr-4 text-zinc-400">${l.channel}</td>
              <td class="py-2.5 pr-4 text-right font-mono text-zinc-300">${(l.input_tokens + l.output_tokens).toLocaleString()}</td>
              <td class="py-2.5 pr-4 text-right font-mono text-zinc-300">${l.ttft_ms ? l.ttft_ms + 'ms' : '—'}</td>
              <td class="py-2.5 pr-4 text-right font-mono text-zinc-300">${(l.duration_ms / 1000).toFixed(1)}s</td>
              <td class="py-2.5 pr-4 text-right font-mono font-semibold ${l.status === 'success' ? 'text-cyan-400' : 'text-zinc-600'}">$${l.total_cost}</td>
              <td class="py-2.5 text-right">${l.status === 'success' ? Badge('成功', 'success') : Badge(l.error_code || '失败', 'error')}</td>
            </tr>`).join('')}
        </tbody>
      </table>
    </div>`;
}

function channelListHTML(channels) {
  if (!channels || !channels.length) return `<div class="py-10 text-center text-xs text-zinc-500">暂无渠道</div>`;
  return channels.map((c) => {
    const disabled = c.status === 0;
    const warn = c.health < 95;
    const circuitBadge = disabled
      ? Badge('已停用', 'neutral')
      : c.circuit === 'half-open' ? Badge('半开探测', 'warning')
      : Badge('熔断关闭', 'success');
    const balance = c.balance == null ? '不限' : `$${Number(c.balance).toFixed(4)}`;
    return `
      <div class="group rounded-lg border border-transparent p-2 -m-2 transition hover:border-zinc-800 hover:bg-zinc-800/40 ${disabled ? 'opacity-45' : ''}">
        <div class="mb-1.5 flex items-center gap-2">
          <span class="h-1.5 w-1.5 rounded-full ${disabled ? 'bg-zinc-500' : warn ? 'bg-amber-400' : 'bg-emerald-400 dot-live text-emerald-400'}"></span>
          <span class="flex-1 truncate text-xs font-semibold">${c.name}</span>
          ${circuitBadge}
        </div>
        ${ProgressBar({ value: c.health, warn, label: `健康度 · 权重 ${c.weight} / 优先级 ${c.priority}`, right: c.health.toFixed(1) + '%' })}
        <div class="mt-1.5 flex items-center justify-between font-mono text-[10px] text-zinc-500">
          <span>${c.rpm} rpm</span>
          <span>余额 ${balance}</span>
          <span>$${c.cost_24h.toFixed(2)} / 24h</span>
        </div>
      </div>`;
  }).join('');
}

function usersListHTML(users) {
  if (!users || !users.length) return `<div class="py-10 text-center text-xs text-zinc-500">暂无用户</div>`;
  return users.map((u) => `
    <div class="row-hover flex items-center gap-3 rounded-lg px-2 py-2 -mx-2">
      <span class="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-gradient-to-br from-zinc-600 to-zinc-800 text-[11px] font-bold text-zinc-200">${String(u.name).slice(0, 2).toUpperCase()}</span>
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-2">
          <span class="truncate text-xs font-semibold">${u.name}</span>
          ${u.group === 'vip' ? Badge('VIP', 'cyan') : u.group === 'free' ? Badge('免费', 'neutral') : ''}
          ${u.status !== 'active' ? Badge('冻结', 'error') : ''}
        </div>
        <div class="mt-0.5 font-mono text-[10px] text-zinc-500">
          可用 <span class="text-zinc-300">$${u.available_balance}</span> · 冻结 $${u.frozen_balance} · 今日 ${u.qpd.toLocaleString()} 次
        </div>
      </div>
    </div>`).join('');
}

function progressListHTML(items) {
  if (!items || !items.length) return `<div class="py-6 text-center text-xs text-zinc-500">暂无数据</div>`;
  return items.map((r) => ProgressBar({
    value: r.usage,
    warn: r.usage >= 90,
    label: `<span class="font-semibold text-zinc-300">${r.target}</span> <span class="text-zinc-600">· ${r.scope}</span>`,
    right: r.usage + '%',
  })).join('');
}

function pricingTableHTML(rows) {
  if (!rows || !rows.length) return `<div class="py-10 text-center text-xs text-zinc-500">暂无定价记录</div>`;
  return `
    <div class="overflow-x-auto">
      <table class="w-full text-left text-xs">
        <thead>
          <tr class="border-b border-zinc-800 text-[11px] uppercase tracking-wide text-zinc-500">
            <th class="pb-2.5 pr-4 font-medium">渠道</th>
            <th class="pb-2.5 pr-4 font-medium">对外模型</th>
            <th class="pb-2.5 pr-4 font-medium">上游模型</th>
            <th class="pb-2.5 pr-4 font-medium text-right">输入 / 1M</th>
            <th class="pb-2.5 pr-4 font-medium text-right">输出 / 1M</th>
            <th class="pb-2.5 pr-4 font-medium text-right">缓存输入 / 1M</th>
            <th class="pb-2.5 font-medium text-right">币种</th>
          </tr>
        </thead>
        <tbody>
          ${rows.map((p) => `
            <tr class="row-hover border-b border-zinc-800/50">
              <td class="py-2.5 pr-4">${p.channel_name || ('#' + p.channel_id)}</td>
              <td class="py-2.5 pr-4 font-semibold">${p.model_name}</td>
              <td class="py-2.5 pr-4 font-mono text-[10px] text-zinc-500">${p.upstream_model || '—'}</td>
              <td class="py-2.5 pr-4 text-right font-mono text-zinc-300">$${p.input_price_per_1m}</td>
              <td class="py-2.5 pr-4 text-right font-mono text-zinc-300">$${p.output_price_per_1m}</td>
              <td class="py-2.5 pr-4 text-right font-mono text-zinc-500">${p.cached_input_price_per_1m ? '$' + p.cached_input_price_per_1m : '—'}</td>
              <td class="py-2.5 text-right text-zinc-400">${p.currency}</td>
            </tr>`).join('')}
        </tbody>
      </table>
    </div>`;
}

function keysTableHTML(rows) {
  if (!rows || !rows.length) return `<div class="py-10 text-center text-xs text-zinc-500">暂无 Key</div>`;
  return `
    <div class="overflow-x-auto">
      <table class="w-full text-left text-xs">
        <thead>
          <tr class="border-b border-zinc-800 text-[11px] uppercase tracking-wide text-zinc-500">
            <th class="pb-2.5 pr-4 font-medium">名称</th>
            <th class="pb-2.5 pr-4 font-medium">用户</th>
            <th class="pb-2.5 pr-4 font-medium">前缀</th>
            <th class="pb-2.5 pr-4 font-medium text-right">状态</th>
            <th class="pb-2.5 font-medium text-right">最后使用</th>
          </tr>
        </thead>
        <tbody>
          ${rows.map((k) => `
            <tr class="row-hover border-b border-zinc-800/50">
              <td class="py-2.5 pr-4 font-semibold">${k.key_name}</td>
              <td class="py-2.5 pr-4 text-zinc-400">#${k.user_id}</td>
              <td class="py-2.5 pr-4 font-mono text-[10px] text-zinc-500">${k.prefix}</td>
              <td class="py-2.5 pr-4 text-right">${k.is_active ? Badge('启用', 'success') : Badge('停用', 'neutral')}</td>
              <td class="py-2.5 text-right font-mono text-[10px] text-zinc-500">${k.last_used_at ? shortTime(k.last_used_at) : '—'}</td>
            </tr>`).join('')}
        </tbody>
      </table>
    </div>`;
}

function modelCatalogHTML(rows) {
  if (!rows || !rows.length) return `<div class="py-10 text-center text-xs text-zinc-500">暂无已发布模型</div>`;
  return `<div class="space-y-2">
    ${rows.map((m) => `
      <div class="row-hover flex items-center justify-between rounded-lg border border-zinc-800/60 bg-zinc-950/35 px-3 py-2.5">
        <div class="min-w-0">
          <div class="truncate text-xs font-semibold">${m.model_name}</div>
          <div class="mt-0.5 font-mono text-[10px] text-zinc-500">${(m.channels || []).map((c) => c.channel_name).join(' · ') || '无渠道'}</div>
        </div>
        <div class="text-right">
          <div class="font-mono text-sm font-bold text-white">${m.channel_count ?? (m.channels || []).length}</div>
          <div class="text-[10px] text-zinc-500">可用渠道</div>
        </div>
      </div>`).join('')}
  </div>`;
}

/* ============================================================
   侧边栏 / 顶栏 / 状态
   ============================================================ */
function renderNav() {
  const nav = document.getElementById('nav');
  nav.innerHTML = '';
  NAV_ITEMS.forEach((item) => {
    if (item.group) {
      nav.append(el(`<div class="px-3 pb-1 pt-4 text-[10px] font-semibold uppercase tracking-widest text-zinc-500">${item.group}</div>`));
      return;
    }
    const btn = el(`
      <button data-key="${item.key}" class="nav-item flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium text-zinc-400">
        ${Icon(item.icon, 'h-4 w-4')}
        <span class="flex-1 text-left">${item.name}</span>
        ${item.badge ? `<span class="rounded-md bg-cyan-400/10 px-1.5 py-0.5 font-mono text-[10px] font-semibold text-cyan-400">${item.badge}</span>` : ''}
      </button>`);
    btn.addEventListener('click', () => navigate(item.key));
    nav.append(btn);
  });
}

function setActiveNav(key) {
  document.querySelectorAll('.nav-item').forEach((n) => n.classList.toggle('active', n.dataset.key === key));
}

function renderChrome(view, key) {
  document.getElementById('breadcrumb').innerHTML = `
    <span class="text-zinc-400">MyApi</span>
    <svg class="h-3.5 w-3.5 text-zinc-600" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="m9 18 6-6-6-6"/></svg>
    <span class="text-zinc-400">Gateway</span>
    <svg class="h-3.5 w-3.5 text-zinc-600" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="m9 18 6-6-6-6"/></svg>
    <span class="font-semibold">${view.title}</span>`;

  document.getElementById('page-title').textContent = view.title;
  document.getElementById('page-subtitle').innerHTML = view.subtitle ? view.subtitle() : '';

  const actions = document.getElementById('page-actions');
  actions.innerHTML = '';
  if (key === 'docs') {
    actions.append(el(`<button class="btn btn-ghost flex items-center gap-1.5 rounded-lg border border-zinc-800 px-3.5 py-2 text-xs font-semibold text-zinc-400">${Icon('dashboard', 'h-3.5 w-3.5')}返回控制台</button>`));
    actions.lastChild.addEventListener('click', () => navigate('dashboard'));
  } else {
    const docsBtn = el(`<button class="btn btn-ghost flex items-center gap-1.5 rounded-lg border border-zinc-800 px-3.5 py-2 text-xs font-semibold text-zinc-400">${Icon('book', 'h-3.5 w-3.5')}查看 API 文档</button>`);
    docsBtn.addEventListener('click', () => navigate('docs'));
    const newBtn = el(`<button class="btn btn-primary flex items-center gap-1.5 rounded-lg px-3.5 py-2 text-xs font-bold">${Icon('zap', 'h-3.5 w-3.5')}新建渠道</button>`);
    newBtn.addEventListener('click', () => navigate('channels'));
    actions.append(docsBtn, newBtn);
  }
  setUpdatedAt();
}

function renderGatewayStatus() {
  const p95 = 1820, uptime = '99.98%';
  document.getElementById('gateway-status').innerHTML = `
    <div class="flex items-center gap-2">
      <span class="dot-live inline-block h-2 w-2 rounded-full bg-emerald-400 text-emerald-400"></span>
      <span class="text-xs font-semibold">网关运行正常</span>
    </div>
    <div class="mt-2 grid grid-cols-2 gap-2 text-center">
      <div class="rounded-lg bg-zinc-950/60 py-1.5">
        <div class="font-mono text-xs font-bold text-cyan-400">${p95}ms</div>
        <div class="text-[10px] text-zinc-500">P95 延迟</div>
      </div>
      <div class="rounded-lg bg-zinc-950/60 py-1.5">
        <div class="font-mono text-xs font-bold text-emerald-400">${uptime}</div>
        <div class="text-[10px] text-zinc-500">30 天可用性</div>
      </div>
    </div>`;
}

function setUpdatedAt() {
  const node = document.getElementById('updated-at');
  if (node) node.textContent = LAST_UPDATED || '—';
}

function animateProgress() {
  requestAnimationFrame(() => {
    setTimeout(() => {
      document.querySelectorAll('.progress-fill').forEach((bar) => {
        bar.style.width = bar.dataset.width + '%';
      });
    }, 120);
  });
}

/* ============================================================
   视图
   ============================================================ */
function renderDashboardView() {
  const bento = document.getElementById('bento');
  const successRate = ((OVERVIEW.success_count / OVERVIEW.request_count) * 100).toFixed(1);

  bento.append(
    StatCard({
      spanCls: 'col-span-12 sm:col-span-6 xl:col-span-3',
      label: '总请求量（7 天）',
      value: (OVERVIEW.request_count / 1000).toFixed(1) + 'K',
      delta: 12.4,
      icon: 'zap',
      spark: Sparkline(DAILY_STATS.slice(-7).map((d) => d.requests)),
    }),
    StatCard({
      spanCls: 'col-span-12 sm:col-span-6 xl:col-span-3',
      label: '请求成功率',
      value: successRate,
      unit: '%',
      delta: 0.6,
      icon: 'gauge',
      spark: Sparkline([98.1, 98.4, 98.2, 98.6, 98.5, 98.9, 98.7], { color: '#34d399' }),
    }),
    StatCard({
      spanCls: 'col-span-12 sm:col-span-6 xl:col-span-3',
      label: '消耗 Tokens（7 天）',
      value: (OVERVIEW.total_tokens / 1e6).toFixed(1) + 'M',
      delta: 8.9,
      icon: 'models',
      spark: Sparkline(DAILY_STATS.slice(-7).map((d) => d.cost), { color: '#a78bfa' }),
    }),
    StatCard({
      spanCls: 'col-span-12 sm:col-span-6 xl:col-span-3',
      label: '累计费用（美元）',
      value: '$' + Number(OVERVIEW.total_cost).toFixed(2),
      delta: 15.2,
      icon: 'wallet',
      spark: Sparkline(DAILY_STATS.slice(-7).map((d) => d.cost), { color: '#f59e0b' }),
    }),
  );

  const trend = Card('col-span-12 xl:col-span-8', `
    ${CardHeader({
      title: '请求趋势（近 14 天）',
      desc: '按 user_daily_stats 自然日汇总 · 含成功 / 失败',
    })}
    <div class="mb-5 flex items-center gap-6 text-xs text-zinc-400">
      <span class="flex items-center gap-1.5"><span class="h-2 w-2 rounded-full bg-cyan-400"></span>今日 <b class="stat-value text-sm text-white">${DAILY_STATS.at(-1).requests.toLocaleString()}</b> 次</span>
      <span class="flex items-center gap-1.5"><span class="h-2 w-2 rounded-full bg-emerald-400"></span>成功率 <b class="text-sm text-white">${successRate}%</b></span>
      <span class="flex items-center gap-1.5"><span class="h-2 w-2 rounded-full bg-amber-400"></span>错误 <b class="text-sm text-white">${OVERVIEW.error_count.toLocaleString()}</b> 次</span>
    </div>`);
  trend.append(BarChart(DAILY_STATS));
  bento.append(trend);

  const docsCard = Card('col-span-12 xl:col-span-4', `
    ${CardHeader({
      title: '看 API 文档',
      desc: '下游兼容接口与管理端接口速查',
      action: `<button data-doc="__list__" class="doc-open btn btn-ghost flex items-center gap-1 rounded-lg border border-zinc-800 px-2.5 py-1.5 text-[11px] font-semibold text-zinc-400">全部文档 ${Icon('book', 'h-3 w-3')}</button>`,
    })}
    <div class="grid gap-2.5">
      ${API_DOCS.map((doc) => `
        <button data-doc="${doc.href}" class="doc-open group flex w-full items-center gap-3 rounded-xl border border-zinc-800/70 bg-zinc-950/45 p-3 text-left transition hover:border-cyan-400/40 hover:bg-cyan-400/5">
          <span class="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-cyan-400/10 text-cyan-400 group-hover:shadow-[0_0_18px_-6px_rgba(34,211,238,.9)]">${Icon(doc.icon, 'h-4 w-4')}</span>
          <span class="min-w-0 flex-1">
            <span class="block truncate text-xs font-semibold text-white">${doc.title}</span>
            <span class="mt-0.5 block truncate font-mono text-[10px] text-zinc-500">${doc.desc}</span>
          </span>
          ${Icon('external', 'h-3.5 w-3.5 text-zinc-600 group-hover:text-cyan-400')}
        </button>`).join('')}
    </div>`);
  bindDocOpeners(docsCard);
  bento.append(docsCard);

  const dist = Card('col-span-12 xl:col-span-4', `${CardHeader({ title: '模型用量分布', desc: '按对外模型名聚合 · 万 tokens' })}`);
  dist.append(DonutChart(MODEL_DIST));
  bento.append(dist);

  const chCard = Card('col-span-12 xl:col-span-4', `
    ${CardHeader({ title: '渠道健康状态', desc: '路由 · 熔断 · 成功率' })}
    <div class="space-y-4">${channelListHTML(CHANNELS)}</div>`);
  bento.append(chCard);

  const routeCard = Card('col-span-12 xl:col-span-4', `
    ${CardHeader({ title: '路由策略雷达', desc: '权重优先级 · 余额过滤 · 熔断探测' })}
    <div class="mb-5 rounded-xl border border-cyan-400/20 bg-gradient-to-br from-cyan-400/10 via-blue-500/5 to-transparent p-4">
      <div class="flex items-center gap-3">
        <span class="grid h-10 w-10 place-items-center rounded-xl bg-cyan-400/10 text-cyan-400">${Icon('route', 'h-5 w-5')}</span>
        <div>
          <div class="font-mono text-2xl font-extrabold text-white">${CHANNELS.filter((c) => c.status === 1).length}</div>
          <div class="text-xs text-zinc-400">当前启用渠道</div>
        </div>
      </div>
    </div>
    <div class="space-y-4">${ROUTING_OVERVIEW.map((r) => ProgressBar({ value: r.value, label: r.label, right: r.right, warn: r.value < 30 })).join('')}</div>`);
  bento.append(routeCard);

  const logCard = Card('col-span-12 xl:col-span-8', `
    ${CardHeader({ title: '实时请求日志', desc: 'usage_logs · 预冻结 → 按实际 usage 结算' })}
    ${logsTableHTML(RECENT_LOGS)}`);
  bento.append(logCard);

  const errorCard = Card('col-span-12 xl:col-span-4', `
    ${CardHeader({ title: '错误画像', desc: '近 7 天非成功请求聚合' })}
    <div class="space-y-2.5">
      ${ERROR_MIX.map((e) => `
        <div class="row-hover flex items-center justify-between rounded-lg border border-zinc-800/60 bg-zinc-950/35 px-3 py-2.5">
          <div class="min-w-0">
            <div class="truncate font-mono text-xs text-zinc-300">${e.name}</div>
            <div class="mt-0.5 text-[10px] text-zinc-500">OpenAI 风格错误码透传</div>
          </div>
          <div class="text-right">
            <div class="font-mono text-sm font-bold text-white">${e.count}</div>
            ${Badge(e.tone === 'warning' ? '限流' : e.tone === 'error' ? '上游' : e.tone === 'cyan' ? '渠道' : '余额', e.tone)}
          </div>
        </div>`).join('')}
    </div>`);
  bento.append(errorCard);

  const rlCard = Card('col-span-12 md:col-span-6 xl:col-span-4', `
    ${CardHeader({ title: '限流配额水位', desc: 'rate_limit_rules · 当前消耗占比' })}
    <div class="space-y-4">${progressListHTML(RATE_LIMITS)}</div>`);
  bento.append(rlCard);

  const userCard = Card('col-span-12 md:col-span-6 xl:col-span-4', `
    ${CardHeader({ title: '用户余额 Top', desc: 'user_balances · 可用 / 冻结（美元）' })}
    <div class="space-y-1">${usersListHTML(TOP_USERS)}</div>`);
  bento.append(userCard);

  const sysCard = Card('col-span-12 xl:col-span-4', `
    ${CardHeader({ title: '系统事件', desc: '熔断 · 结算 · 路由变更' })}
    <ul class="space-y-3.5">
      ${NOTIFICATIONS.map((n) => `
        <li class="flex items-start gap-3">
          <span class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full ${n.tone === 'warning' ? 'bg-amber-400' : n.tone === 'success' ? 'bg-emerald-400' : 'bg-cyan-400'}"></span>
          <div class="text-xs leading-relaxed text-zinc-400">${n.text}</div>
        </li>`).join('')}
    </ul>`);
  bento.append(sysCard);

  animateProgress();
}

function renderUsageView() {
  const bento = document.getElementById('bento');
  const successRate = ((OVERVIEW.success_count / OVERVIEW.request_count) * 100).toFixed(1);
  bento.append(
    StatCard({ spanCls: 'col-span-12 sm:col-span-6 xl:col-span-3', label: '请求总量', value: (OVERVIEW.request_count / 1000).toFixed(1) + 'K', delta: 0, deltaLabel: '近 7 天', icon: 'zap', spark: Sparkline(DAILY_STATS.slice(-7).map((d) => d.requests)) }),
    StatCard({ spanCls: 'col-span-12 sm:col-span-6 xl:col-span-3', label: '成功率', value: successRate, unit: '%', delta: 0, deltaLabel: '近 7 天', icon: 'gauge', spark: Sparkline(DAILY_STATS.slice(-7).map((d) => d.success), { color: '#34d399' }) }),
    StatCard({ spanCls: 'col-span-12 sm:col-span-6 xl:col-span-3', label: 'Tokens', value: (OVERVIEW.total_tokens / 1e6).toFixed(2) + 'M', delta: 0, deltaLabel: '近 7 天', icon: 'models', spark: Sparkline(DAILY_STATS.slice(-7).map((d) => d.tokens), { color: '#a78bfa' }) }),
    StatCard({ spanCls: 'col-span-12 sm:col-span-6 xl:col-span-3', label: '费用（美元）', value: '$' + Number(OVERVIEW.total_cost).toFixed(2), delta: 0, deltaLabel: '近 7 天', icon: 'wallet', spark: Sparkline(DAILY_STATS.slice(-7).map((d) => d.cost), { color: '#f59e0b' }) }),
  );

  const trend = Card('col-span-12 xl:col-span-8', `${CardHeader({ title: '请求趋势（近 14 天）', desc: 'user_daily_stats · 请求次数 / 费用' })}`);
  trend.append(BarChart(DAILY_STATS));
  bento.append(trend);

  const dist = Card('col-span-12 xl:col-span-4', `${CardHeader({ title: '模型用量分布', desc: 'usage_logs 聚合 · 万 tokens' })}`);
  dist.append(DonutChart(MODEL_DIST));
  bento.append(dist);

  const dailyCard = Card('col-span-12', `
    ${CardHeader({ title: '每日明细', desc: '近 14 天' })}
    <div class="overflow-x-auto">
      <table class="w-full text-left text-xs">
        <thead><tr class="border-b border-zinc-800 text-[11px] uppercase tracking-wide text-zinc-500">
          <th class="pb-2.5 pr-4 font-medium">日期</th>
          <th class="pb-2.5 pr-4 font-medium text-right">请求数</th>
          <th class="pb-2.5 pr-4 font-medium text-right">成功</th>
          <th class="pb-2.5 pr-4 font-medium text-right">失败</th>
          <th class="pb-2.5 pr-4 font-medium text-right">Tokens</th>
          <th class="pb-2.5 font-medium text-right">费用</th>
        </tr></thead>
        <tbody>
          ${DAILY_STATS.map((d) => `
            <tr class="row-hover border-b border-zinc-800/50">
              <td class="py-2 pr-4 font-mono text-zinc-400">${d.date}</td>
              <td class="py-2 pr-4 text-right font-mono text-zinc-300">${d.requests.toLocaleString()}</td>
              <td class="py-2 pr-4 text-right font-mono text-emerald-400">${d.success.toLocaleString()}</td>
              <td class="py-2 pr-4 text-right font-mono text-amber-400">${d.errors.toLocaleString()}</td>
              <td class="py-2 pr-4 text-right font-mono text-zinc-300">${d.tokens.toLocaleString()}</td>
              <td class="py-2 text-right font-mono font-semibold text-cyan-400">$${d.cost.toFixed(4)}</td>
            </tr>`).join('')}
        </tbody>
      </table>
    </div>`);
  bento.append(dailyCard);
  animateProgress();
}

function renderLogsView() {
  const bento = document.getElementById('bento');
  const card = Card('col-span-12', `
    ${CardHeader({ title: '请求日志', desc: 'usage_logs · 最近记录（含成功/失败）' })}
    ${logsTableHTML(RECENT_LOGS)}`);
  bento.append(card);
}

function renderChannelsView() {
  const bento = document.getElementById('bento');
  const card = Card('col-span-12 xl:col-span-8', `
    ${CardHeader({ title: '渠道列表', desc: 'channels · 状态 / 权重 / 优先级 / 余额' })}
    <div class="space-y-4">${channelListHTML(CHANNELS)}</div>`);
  bento.append(card);

  const side = Card('col-span-12 xl:col-span-4', `
    ${CardHeader({ title: '路由概览', desc: '渠道健康与模型发布' })}
    <div class="space-y-4">${ROUTING_OVERVIEW.map((r) => ProgressBar({ value: r.value, label: r.label, right: r.right, warn: r.value < 30 })).join('')}</div>
    <div class="mt-5 grid grid-cols-2 gap-2 text-center">
      <div class="rounded-lg border border-zinc-800 bg-zinc-950/45 p-3">
        <div class="font-mono text-lg font-bold text-emerald-400">${CHANNELS.filter((c) => c.status === 1).length}</div>
        <div class="text-[10px] text-zinc-500">启用渠道</div>
      </div>
      <div class="rounded-lg border border-zinc-800 bg-zinc-950/45 p-3">
        <div class="font-mono text-lg font-bold text-amber-400">${CHANNELS.filter((c) => c.health < 95).length}</div>
        <div class="text-[10px] text-zinc-500">需关注渠道</div>
      </div>
    </div>`);
  bento.append(side);
  animateProgress();
}

function renderModelsView() {
  const bento = document.getElementById('bento');
  const catalog = Card('col-span-12 xl:col-span-7', `
    ${CardHeader({ title: '已发布模型目录', desc: 'GET /admin/models · 对外模型名与可用渠道' })}
    ${modelCatalogHTML(MODEL_CATALOG)}`);
  bento.append(catalog);

  const dist = Card('col-span-12 xl:col-span-5', `${CardHeader({ title: '模型用量分布', desc: '万 tokens' })}`);
  dist.append(DonutChart(MODEL_DIST));
  bento.append(dist);

  const routing = Card('col-span-12', `
    ${CardHeader({ title: '路由策略', desc: '权重优先级 · 余额过滤 · 熔断探测' })}
    <div class="space-y-4">${ROUTING_OVERVIEW.map((r) => ProgressBar({ value: r.value, label: r.label, right: r.right, warn: r.value < 30 })).join('')}</div>`);
  bento.append(routing);
  animateProgress();
}

async function renderUsersView() {
  const bento = document.getElementById('bento');
  bento.append(Card('col-span-12 xl:col-span-6', `
    ${CardHeader({ title: '用户余额 Top', desc: 'user_balances · 可用 / 冻结（美元）' })}
    <div class="space-y-1">${usersListHTML(TOP_USERS)}</div>`));

  const keyCard = Card('col-span-12 xl:col-span-6', `
    ${CardHeader({ title: '网关 Key', desc: 'client_api_keys · 仅存哈希，明文仅下发一次' })}
    <div id="keys-body" class="py-10 text-center text-xs text-zinc-500">加载中…</div>`);
  bento.append(keyCard);

  try {
    const data = await adminGet('/keys', { page: 1, page_size: 20 });
    document.getElementById('keys-body').innerHTML = keysTableHTML(data?.list || []);
  } catch (err) {
    document.getElementById('keys-body').innerHTML = `<div class="py-10 text-center text-xs text-rose-400">加载失败：${err.message}</div>`;
  }
}

function renderLimitsView() {
  const bento = document.getElementById('bento');
  bento.append(Card('col-span-12', `
    ${CardHeader({ title: '限流配额水位', desc: 'rate_limit_rules · rpm / tpm / tpd / 并发' })}
    <div class="space-y-4">${progressListHTML(RATE_LIMITS)}</div>`));

  bento.append(Card('col-span-12', `
    ${CardHeader({ title: '规则明细', desc: '来自 /admin/rate-limits' })}
    <div class="overflow-x-auto">
      <table class="w-full text-left text-xs">
        <thead><tr class="border-b border-zinc-800 text-[11px] uppercase tracking-wide text-zinc-500">
          <th class="pb-2.5 pr-4 font-medium">作用域</th>
          <th class="pb-2.5 pr-4 font-medium">目标</th>
          <th class="pb-2.5 pr-4 font-medium">指标</th>
          <th class="pb-2.5 font-medium text-right">限额</th>
        </tr></thead>
        <tbody>
          ${RATE_LIMITS.map((r) => `
            <tr class="row-hover border-b border-zinc-800/50">
              <td class="py-2 pr-4 text-zinc-400">${r.scope}</td>
              <td class="py-2 pr-4 font-semibold">${r.target}</td>
              <td class="py-2 pr-4 font-mono text-cyan-400">${r.metric || '—'}</td>
              <td class="py-2 text-right font-mono text-zinc-300">${Number(r.limit_value || 0).toLocaleString()}</td>
            </tr>`).join('')}
        </tbody>
      </table>
    </div>`));
  animateProgress();
}

async function renderPricingView() {
  const bento = document.getElementById('bento');
  const card = Card('col-span-12', `
    ${CardHeader({ title: '渠道×模型定价', desc: 'model_pricing · 单位：美元 / 1M tokens' })}
    <div id="pricing-body" class="py-10 text-center text-xs text-zinc-500">加载中…</div>`);
  bento.append(card);
  try {
    const data = await adminGet('/pricing', { page: 1, page_size: 100 });
    document.getElementById('pricing-body').innerHTML = pricingTableHTML(data?.list || []);
  } catch (err) {
    document.getElementById('pricing-body').innerHTML = `<div class="py-10 text-center text-xs text-rose-400">加载失败：${err.message}</div>`;
  }
}

function renderSettingsView() {
  const bento = document.getElementById('bento');
  const base = dashboardApiBase();
  bento.append(Card('col-span-12 xl:col-span-6', `
    ${CardHeader({ title: '运行信息', desc: '当前控制台接入信息' })}
    <div class="space-y-3 text-xs">
      <div class="flex items-center justify-between rounded-lg border border-zinc-800 bg-zinc-950/40 px-3 py-2.5">
        <span class="text-zinc-400">管理端接口地址</span><span class="font-mono text-cyan-400">${base}</span>
      </div>
      <div class="flex items-center justify-between rounded-lg border border-zinc-800 bg-zinc-950/40 px-3 py-2.5">
        <span class="text-zinc-400">健康检查</span><a class="font-mono text-cyan-400 hover:underline" href="../healthz" target="_blank">/healthz</a>
      </div>
      <div class="flex items-center justify-between rounded-lg border border-zinc-800 bg-zinc-950/40 px-3 py-2.5">
        <span class="text-zinc-400">数据更新时间</span><span class="font-mono text-zinc-300">${LAST_UPDATED || '—'}</span>
      </div>
    </div>`));

  bento.append(Card('col-span-12 xl:col-span-6', `
    ${CardHeader({ title: '接口说明', desc: '管理端当前不设认证（内网部署）' })}
    <ul class="space-y-2 text-xs text-zinc-400">
      <li>· 跨端口部署可用 <span class="font-mono text-cyan-400">?api_base=http://host:port/admin</span> 指定接口地址</li>
      <li>· 管理端接口返回 <span class="font-mono text-cyan-400">{code, message, data}</span>，code=0 为成功</li>
      <li>· 下游接口为 OpenAI 兼容协议，鉴权使用网关 Key</li>
    </ul>`));
}

async function renderDocsView(docHref) {
  const bento = document.getElementById('bento');
  const active = docHref || DOC_LIST[0].href;

  const list = Card('col-span-12 xl:col-span-3', `
    ${CardHeader({ title: '文档目录', desc: '点击切换' })}
    <div id="doc-list" class="space-y-1">
      ${DOC_LIST.map((d) => `
        <button data-href="${d.href}" class="doc-link w-full rounded-lg px-3 py-2 text-left text-xs transition ${d.href === active ? 'bg-cyan-400/10 font-semibold text-cyan-400' : 'text-zinc-400 hover:bg-zinc-800/60'}">${d.title}</button>`).join('')}
    </div>`);
  bento.append(list);

  const body = Card('col-span-12 xl:col-span-9', `
    <div class="mb-4 flex items-center justify-between gap-3">
      <div class="min-w-0">
        <div id="doc-title" class="text-sm font-semibold">${(DOC_LIST.find((d) => d.href === active) || DOC_LIST[0]).title}</div>
        <div id="doc-path" class="mt-0.5 truncate font-mono text-[11px] text-zinc-500">${active}</div>
      </div>
      <button id="doc-back" class="btn btn-ghost flex shrink-0 items-center gap-1.5 rounded-lg border border-zinc-800 px-3 py-1.5 text-[11px] font-semibold text-zinc-400">${Icon('dashboard', 'h-3 w-3')}返回控制台</button>
    </div>
    <article id="doc-content" class="md-body">加载中…</article>`);
  bento.append(body);

  list.querySelectorAll('.doc-link').forEach((btn) => {
    btn.addEventListener('click', () => navigate('docs', btn.dataset.href));
  });
  document.getElementById('doc-back').addEventListener('click', () => navigate('dashboard'));

  try {
    const res = await fetch(active, { headers: { Accept: 'text/markdown, text/plain, */*' } });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const text = await res.text();
    const html = window.marked ? window.marked.parse(text) : `<pre>${text}</pre>`;
    const clean = window.DOMPurify ? window.DOMPurify.sanitize(html) : html;
    const node = document.getElementById('doc-content');
    if (node) node.innerHTML = clean;
  } catch (err) {
    const node = document.getElementById('doc-content');
    if (node) node.innerHTML = `<div class="py-6 text-rose-400">文档加载失败：${err.message}</div>`;
  }
}

function bindDocOpeners(scope) {
  scope.querySelectorAll('.doc-open').forEach((btn) => {
    btn.addEventListener('click', () => {
      const href = btn.dataset.doc;
      navigate('docs', href === '__list__' ? null : href);
    });
  });
}

/* ============================================================
   视图注册表 / 路由
   ============================================================ */
const VIEWS = {
  dashboard: { title: '运营仪表盘', subtitle: () => `网关运行总览 · 数据更新于 <span id="updated-at" class="font-mono text-cyan-400">${LAST_UPDATED || '—'}</span>`, render: renderDashboardView },
  usage:     { title: '用量统计', subtitle: () => '按用户 / 模型聚合的 token 与费用', render: renderUsageView },
  logs:      { title: '请求日志', subtitle: () => 'usage_logs 明细', render: renderLogsView },
  docs:      { title: 'API 文档', subtitle: () => '下游与管理端接口文档（应用内阅读）', render: renderDocsView },
  channels:  { title: '渠道管理', subtitle: () => '上游渠道 · 健康 / 权重 / 余额', render: renderChannelsView },
  models:    { title: '模型路由', subtitle: () => '对外模型目录与路由策略', render: renderModelsView },
  users:     { title: '用户与 Key', subtitle: () => '下游用户余额与网关 Key', render: renderUsersView },
  limits:    { title: '限流规则', subtitle: () => 'rate_limit_rules · 配额水位', render: renderLimitsView },
  pricing:   { title: '计费定价', subtitle: () => '渠道×模型单价', render: renderPricingView },
  settings:  { title: '系统设置', subtitle: () => '控制台与网关信息', render: renderSettingsView },
};

function parseRoute() {
  const raw = window.location.hash.replace(/^#\/?/, '');
  const [key, qs] = raw.split('?');
  const params = new URLSearchParams(qs || '');
  return { key: key || 'dashboard', doc: params.get('doc') };
}

function applyRoute() {
  const { key, doc } = parseRoute();
  const activeKey = VIEWS[key] ? key : 'dashboard';
  const view = VIEWS[activeKey];
  setActiveNav(activeKey);
  renderChrome(view, activeKey);
  const bento = document.getElementById('bento');
  bento.innerHTML = '';
  view.render(doc);
}

function navigate(key, doc) {
  const target = '#/' + key + (doc ? '?doc=' + encodeURIComponent(doc) : '');
  if (window.location.hash === target) {
    applyRoute();
  } else {
    window.location.hash = target;
  }
}

window.addEventListener('hashchange', applyRoute);

/* ============================================================
   交互
   ============================================================ */
function bindInteractions() {
  document.getElementById('theme-toggle').addEventListener('click', () => {
    document.documentElement.classList.toggle('dark');
  });

  const bell = document.getElementById('notify-btn');
  const panel = el(`
    <div id="notify-panel" class="absolute right-0 top-11 z-50 hidden w-80 rounded-xl border border-zinc-800 bg-zinc-900 p-3 shadow-2xl">
      <div class="mb-2 px-1 text-xs font-semibold text-zinc-400">通知（${NOTIFICATIONS.length}）</div>
      ${NOTIFICATIONS.map((n) => `
        <div class="row-hover rounded-lg p-2.5 text-xs leading-relaxed text-zinc-400">
          ${Badge(n.tone === 'warning' ? '警告' : n.tone === 'success' ? '结算' : '路由', n.tone)}
          <div class="mt-1.5">${n.text}</div>
        </div>`).join('')}
    </div>`);
  bell.style.position = 'relative';
  bell.append(panel);
  bell.addEventListener('click', (e) => {
    e.stopPropagation();
    panel.classList.toggle('hidden');
  });
  document.addEventListener('click', () => panel.classList.add('hidden'));

  document.addEventListener('keydown', (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      document.getElementById('global-search').focus();
    }
  });
}

function renderLoading() {
  document.getElementById('bento').innerHTML = `
    <section class="card rounded-xl border border-zinc-800 bg-zinc-900/90 p-6 col-span-12">
      <div class="flex items-center gap-3 text-sm text-zinc-400">
        <span class="h-2 w-2 rounded-full bg-cyan-400 dot-live text-cyan-400"></span>
        正在连接管理端接口并加载实时数据...
      </div>
    </section>`;
}

function renderError(err) {
  document.getElementById('bento').innerHTML = `
    <section class="card rounded-xl border border-amber-400/30 bg-zinc-900/90 p-6 col-span-12">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <div class="text-sm font-semibold text-amber-400">管理端接口连接失败</div>
          <p class="mt-1 text-xs text-zinc-400">请确认网关服务已启动，且 Dashboard 与 /admin 接口同源；跨端口部署时可使用 <span class="font-mono text-cyan-400">?api_base=http://host:port/admin</span> 指定接口地址。</p>
          <p class="mt-2 font-mono text-xs text-zinc-500">${String(err.message || err)}</p>
        </div>
        <button onclick="window.location.reload()" class="btn btn-primary rounded-lg px-3.5 py-2 text-xs font-bold">重新加载</button>
      </div>
    </section>`;
}

/* ---------- 启动 ---------- */
async function bootDashboard() {
  renderNav();
  renderGatewayStatus();
  renderLoading();
  try {
    await loadDashboardData();
    LAST_UPDATED = new Date().toLocaleTimeString('zh-CN', { hour12: false });
    bindInteractions();
    applyRoute();
  } catch (err) {
    renderError(err);
    LAST_UPDATED = '加载失败';
  }
}

bootDashboard();
