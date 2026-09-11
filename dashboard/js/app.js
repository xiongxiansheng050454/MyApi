/* ============================================================
   仪表盘装配与交互
   ============================================================ */

/* ---------- 侧边栏导航 ---------- */
const NAV_ITEMS = [
  { group: '总览' },
  { name: '仪表盘',   icon: 'dashboard', active: true },
  { name: '用量统计', icon: 'chart' },
  { name: '请求日志', icon: 'logs', badge: '1.7k' },
  { name: 'API 文档', icon: 'book' },
  { group: '资源管理' },
  { name: '渠道管理', icon: 'channels' },
  { name: '模型路由', icon: 'models' },
  { name: '用户与 Key', icon: 'users' },
  { group: '运营' },
  { name: '限流规则', icon: 'gauge' },
  { name: '计费定价', icon: 'wallet' },
  { name: '系统设置', icon: 'settings' },
];

function renderNav() {
  const nav = document.getElementById('nav');
  NAV_ITEMS.forEach((item) => {
    if (item.group) {
      nav.append(el(`<div class="px-3 pb-1 pt-4 text-[10px] font-semibold uppercase tracking-widest text-zinc-500">${item.group}</div>`));
      return;
    }
    const btn = el(`
      <button class="nav-item flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium text-zinc-400 ${item.active ? 'active' : ''}">
        ${Icon(item.icon, 'h-4 w-4')}
        <span class="flex-1 text-left">${item.name}</span>
        ${item.badge ? `<span class="rounded-md bg-cyan-400/10 px-1.5 py-0.5 font-mono text-[10px] font-semibold text-cyan-400">${item.badge}</span>` : ''}
      </button>`);
    btn.addEventListener('click', () => {
      nav.querySelectorAll('.nav-item').forEach((n) => n.classList.remove('active'));
      btn.classList.add('active');
    });
    nav.append(btn);
  });
}

/* ---------- 面包屑 / 页面操作 ---------- */
function renderChrome() {
  document.getElementById('breadcrumb').innerHTML = `
    <span class="text-zinc-400">MyApi</span>
    <svg class="h-3.5 w-3.5 text-zinc-600" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="m9 18 6-6-6-6"/></svg>
    <span class="text-zinc-400">Gateway</span>
    <svg class="h-3.5 w-3.5 text-zinc-600" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="m9 18 6-6-6-6"/></svg>
    <span class="font-semibold">仪表盘</span>`;

  const actions = document.getElementById('page-actions');
  actions.append(
    el(`<a href="../docs/README.md" class="btn btn-ghost rounded-lg border border-zinc-800 px-3.5 py-2 text-xs font-semibold text-zinc-400">查看 API 文档</a>`),
    el(`<button class="btn btn-primary flex items-center gap-1.5 rounded-lg px-3.5 py-2 text-xs font-bold">${Icon('zap', 'h-3.5 w-3.5')}新建渠道</button>`),
  );
}

/* ---------- 侧边栏底部：网关状态 ---------- */
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

/* ---------- Bento 模块 ---------- */
function renderBento() {
  const bento = document.getElementById('bento');
  const successRate = ((OVERVIEW.success_count / OVERVIEW.request_count) * 100).toFixed(1);

  /* Row 1 · KPI 总览卡（对齐 /admin/stats/overview） */
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

  /* Row 2 · 请求趋势（8 列） + API 文档（4 列） */
  const trend = Card('col-span-12 xl:col-span-8', `
    ${CardHeader({
      title: '请求趋势（近 14 天）',
      desc: '按 user_daily_stats 自然日汇总 · 含成功 / 失败',
      action: `<div class="flex gap-1 rounded-lg border border-zinc-800 p-0.5 text-[11px] font-medium">
        <span class="rounded-md bg-cyan-400/10 px-2.5 py-1 text-cyan-400">14 天</span>
        <span class="cursor-pointer rounded-md px-2.5 py-1 text-zinc-500 hover:text-cyan-400">30 天</span>
        <span class="cursor-pointer rounded-md px-2.5 py-1 text-zinc-500 hover:text-cyan-400">90 天</span>
      </div>`,
    })}
    <div class="mb-5 flex items-center gap-6 text-xs text-zinc-400">
      <span class="flex items-center gap-1.5"><span class="h-2 w-2 rounded-full bg-cyan-400"></span>今日 <b class="stat-value text-sm text-white">${DAILY_STATS.at(-1).requests.toLocaleString()}</b> 次</span>
      <span class="flex items-center gap-1.5"><span class="h-2 w-2 rounded-full bg-emerald-400"></span>成功率 <b class="text-sm text-white">${successRate}%</b></span>
      <span class="flex items-center gap-1.5"><span class="h-2 w-2 rounded-full bg-amber-400"></span>错误 <b class="text-sm text-white">${OVERVIEW.error_count.toLocaleString()}</b> 次</span>
    </div>`);
  trend.append(BarChart(DAILY_STATS));
  trend.append(el('<div class="h-4"></div>'));
  bento.append(trend);

  const docsCard = Card('col-span-12 xl:col-span-4', `
    ${CardHeader({
      title: '看 API 文档',
      desc: '下游兼容接口与管理端接口速查',
      action: `<a href="../docs/README.md" class="btn btn-ghost flex items-center gap-1 rounded-lg border border-zinc-800 px-2.5 py-1.5 text-[11px] font-semibold text-zinc-400">全部文档 ${Icon('external', 'h-3 w-3')}</a>`,
    })}
    <div class="grid gap-2.5">
      ${API_DOCS.map((doc) => `
        <a href="${doc.href}" class="group flex items-center gap-3 rounded-xl border border-zinc-800/70 bg-zinc-950/45 p-3 transition hover:border-cyan-400/40 hover:bg-cyan-400/5">
          <span class="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-cyan-400/10 text-cyan-400 group-hover:shadow-[0_0_18px_-6px_rgba(34,211,238,.9)]">${Icon(doc.icon, 'h-4 w-4')}</span>
          <span class="min-w-0 flex-1">
            <span class="block truncate text-xs font-semibold text-white">${doc.title}</span>
            <span class="mt-0.5 block truncate font-mono text-[10px] text-zinc-500">${doc.desc}</span>
          </span>
          ${Icon('external', 'h-3.5 w-3.5 text-zinc-600 group-hover:text-cyan-400')}
        </a>`).join('')}
    </div>`);
  bento.append(docsCard);

  /* Row 3 · 模型分布（4 列） + 渠道健康（4 列） + 路由策略（4 列） */
  const dist = Card('col-span-12 xl:col-span-4', `
    ${CardHeader({ title: '模型用量分布', desc: '按对外模型名聚合 · 万 tokens' })}
  `);
  dist.append(DonutChart(MODEL_DIST));
  bento.append(dist);

  const chCard = Card('col-span-12 xl:col-span-4', `
    ${CardHeader({
      title: '渠道健康状态',
      desc: '路由 · 熔断 · 成功率',
      action: `<button class="btn btn-ghost flex items-center gap-1 rounded-lg border border-zinc-800 px-2.5 py-1.5 text-[11px] font-semibold text-zinc-400">管理渠道 ${Icon('external', 'h-3 w-3')}</button>`,
    })}
    <div id="channel-list" class="space-y-4"></div>`);
  const chList = chCard.querySelector('#channel-list');
  CHANNELS.forEach((c) => {
    const disabled = c.status === 0;
    const warn = c.health < 95;
    const circuitBadge = disabled
      ? Badge('已停用', 'neutral')
      : c.circuit === 'half-open' ? Badge('半开探测', 'warning')
      : Badge('熔断关闭', 'success');
    chList.append(el(`
      <div class="group rounded-lg border border-transparent p-2 -m-2 transition hover:border-zinc-800 hover:bg-zinc-800/40 ${disabled ? 'opacity-45' : ''}">
        <div class="mb-1.5 flex items-center gap-2">
          <span class="h-1.5 w-1.5 rounded-full ${disabled ? 'bg-zinc-500' : warn ? 'bg-amber-400' : 'bg-emerald-400 dot-live text-emerald-400'}"></span>
          <span class="flex-1 truncate text-xs font-semibold">${c.name}</span>
          ${circuitBadge}
        </div>
        ${ProgressBar({ value: c.health, warn, label: `健康度 · 权重 ${c.weight} / 优先级 ${c.priority}`, right: c.health.toFixed(1) + '%' })}
        <div class="mt-1.5 flex items-center justify-between font-mono text-[10px] text-zinc-500">
          <span>${c.rpm} rpm</span>
          <span>${c.models.length} 个模型</span>
          <span>$${c.cost_24h.toFixed(2)} / 24h</span>
        </div>
      </div>`));
  });
  bento.append(chCard);

  const routeCard = Card('col-span-12 xl:col-span-4', `
    ${CardHeader({ title: '路由策略雷达', desc: '权重优先级 · 余额过滤 · 熔断探测' })}
    <div class="mb-5 rounded-xl border border-cyan-400/20 bg-gradient-to-br from-cyan-400/10 via-blue-500/5 to-transparent p-4">
      <div class="flex items-center gap-3">
        <span class="grid h-10 w-10 place-items-center rounded-xl bg-cyan-400/10 text-cyan-400">${Icon('route', 'h-5 w-5')}</span>
        <div>
          <div class="font-mono text-2xl font-extrabold text-white">1,775</div>
          <div class="text-xs text-zinc-400">当前每分钟路由决策</div>
        </div>
      </div>
    </div>
    <div class="space-y-4">
      ${ROUTING_OVERVIEW.map((r) => ProgressBar({ value: r.value, label: r.label, right: r.right, warn: r.value < 30 })).join('')}
    </div>
    <div class="mt-5 grid grid-cols-2 gap-2 text-center">
      <div class="rounded-lg border border-zinc-800 bg-zinc-950/45 p-3">
        <div class="font-mono text-lg font-bold text-emerald-400">3</div>
        <div class="text-[10px] text-zinc-500">候选池</div>
      </div>
      <div class="rounded-lg border border-zinc-800 bg-zinc-950/45 p-3">
        <div class="font-mono text-lg font-bold text-amber-400">1</div>
        <div class="text-[10px] text-zinc-500">半开渠道</div>
      </div>
    </div>`);
  bento.append(routeCard);

  /* Row 4 · 实时请求日志（8 列） + 错误画像（4 列） */

  const logCard = Card('col-span-12 xl:col-span-8', `
    ${CardHeader({
      title: '实时请求日志',
      desc: 'usage_logs · 预冻结 → 按实际 usage 结算',
      action: `<button class="btn btn-ghost flex items-center gap-1 rounded-lg border border-zinc-800 px-2.5 py-1.5 text-[11px] font-semibold text-zinc-400">全部日志 ${Icon('external', 'h-3 w-3')}</button>`,
    })}
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
          ${RECENT_LOGS.map((l) => `
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
              <td class="py-2.5 text-right">${l.status === 'success' ? Badge('成功', 'success') : Badge('失败 · 429', 'error')}</td>
            </tr>`).join('')}
        </tbody>
      </table>
    </div>`);
  bento.append(logCard);

  const errorCard = Card('col-span-12 xl:col-span-4', `
    ${CardHeader({ title: '错误画像', desc: '近 24 小时非成功请求聚合' })}
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

  /* Row 5 · 限流（4）+ 用户余额（4）+ 系统事件（4） */
  const rlCard = Card('col-span-12 md:col-span-6 xl:col-span-4', `
    ${CardHeader({ title: '限流配额水位', desc: 'rate_limit_rules · 当前消耗占比' })}
    <div class="space-y-4">
      ${RATE_LIMITS.map((r) => ProgressBar({
        value: r.usage,
        warn: r.usage >= 90,
        label: `<span class="font-semibold text-zinc-300">${r.target}</span> <span class="text-zinc-600">· ${r.scope}</span>`,
        right: r.usage + '%',
      })).join('')}
    </div>`);
  bento.append(rlCard);

  const userCard = Card('col-span-12 md:col-span-6 xl:col-span-4', `
    ${CardHeader({ title: '用户余额 Top', desc: 'user_balances · 可用 / 冻结（美元）' })}
    <div class="space-y-1">
      ${TOP_USERS.map((u) => `
        <div class="row-hover flex items-center gap-3 rounded-lg px-2 py-2 -mx-2">
          <span class="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-gradient-to-br from-zinc-600 to-zinc-800 text-[11px] font-bold text-zinc-200">${u.name.slice(0, 2).toUpperCase()}</span>
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
        </div>`).join('')}
    </div>`);
  bento.append(userCard);

  const sysCard = Card('col-span-12 xl:col-span-4', `
    ${CardHeader({ title: '系统事件', desc: '熔断 · 结算 · 路由变更' })}
    <ul class="space-y-3.5">
      ${NOTIFICATIONS.map((n) => `
        <li class="flex items-start gap-3">
          <span class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full ${n.tone === 'warning' ? 'bg-amber-400' : n.tone === 'success' ? 'bg-emerald-400' : 'bg-cyan-400'}"></span>
          <div class="text-xs leading-relaxed text-zinc-400">${n.text}</div>
        </li>`).join('')}
    </ul>
    <div class="mt-5 rounded-lg border border-dashed border-zinc-700/60 p-3 text-center">
      <div class="font-mono text-lg font-bold text-cyan-400">${OVERVIEW.active_user_count}</div>
      <div class="text-[10px] text-zinc-500">近 7 天活跃下游用户</div>
    </div>`);
  bento.append(sysCard);
}

/* ---------- 交互 ---------- */
function bindInteractions() {
  // 主题切换
  document.getElementById('theme-toggle').addEventListener('click', () => {
    document.documentElement.classList.toggle('dark');
  });

  // 通知面板
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

  // Cmd/Ctrl+K 聚焦搜索
  document.addEventListener('keydown', (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      document.getElementById('global-search').focus();
    }
  });

  // 进度条入场动画
  requestAnimationFrame(() => {
    setTimeout(() => {
      document.querySelectorAll('.progress-fill').forEach((bar) => {
        bar.style.width = bar.dataset.width + '%';
      });
    }, 150);
  });

  // 更新时间戳
  document.getElementById('updated-at').textContent = new Date().toLocaleTimeString('zh-CN', { hour12: false });
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
  renderChrome();
  renderGatewayStatus();
  renderLoading();
  try {
    await loadDashboardData();
    document.getElementById('bento').innerHTML = '';
    renderBento();
    bindInteractions();
  } catch (err) {
    renderError(err);
    document.getElementById('updated-at').textContent = '加载失败';
  }
}

bootDashboard();
