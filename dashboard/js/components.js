/* ============================================================
   基础组件库
   - el(html)            将 HTML 字符串转为 DOM 元素
   - Card                卡片容器
   - CardHeader          卡片头部（标题 + 操作区）
   - StatCard            大数字统计卡（label + value + 变化趋势）
   - Badge               状态徽标（success / warning / error / neutral / cyan）
   - ProgressBar         进度条
   - Sparkline           SVG 迷你趋势折线
   - BarChart            SVG 柱状趋势图（带悬浮 tooltip）
   - DonutChart          环形占比图（conic-gradient）
   - Icon                SVG 图标库
   ============================================================ */

/** HTML 字符串 -> DOM Element */
function el(html) {
  const t = document.createElement('template');
  t.innerHTML = html.trim();
  return t.content.firstElementChild;
}

/** 图标库（lucide 风格线性图标） */
const Icon = (name, cls = 'h-4 w-4') => {
  const paths = {
    dashboard : '<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>',
    chart     : '<path d="M3 3v18h18"/><path d="M7 15v-4m5 4V8m5 7v-6"/>',
    logs      : '<path d="M8 6h13M8 12h13M8 18h13"/><path d="M3 6h.01M3 12h.01M3 18h.01"/>',
    book      : '<path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H21"/><path d="M4 4.5A2.5 2.5 0 0 1 6.5 2H21v20H6.5A2.5 2.5 0 0 1 4 19.5z"/>',
    shield    : '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10"/><path d="m9 12 2 2 4-4"/>',
    activity  : '<path d="M22 12h-4l-3 8L9 4l-3 8H2"/>',
    route     : '<circle cx="6" cy="19" r="3"/><path d="M9 19h8.5a3.5 3.5 0 0 0 0-7h-11a3.5 3.5 0 0 1 0-7H15"/><circle cx="18" cy="5" r="3"/>',
    key       : '<circle cx="7.5" cy="15.5" r="5.5"/><path d="m21 2-9.6 9.6"/><path d="m15.5 7.5 3 3L22 7l-3-3"/>',
    channels  : '<path d="M4 6h16M4 12h16M4 18h16"/><circle cx="8" cy="6" r="1.6" fill="currentColor" stroke="none"/><circle cx="15" cy="12" r="1.6" fill="currentColor" stroke="none"/><circle cx="9" cy="18" r="1.6" fill="currentColor" stroke="none"/>',
    models    : '<path d="M12 2 2 7l10 5 10-5-10-5z"/><path d="m2 12 10 5 10-5"/><path d="m2 17 10 5 10-5"/>',
    users     : '<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/>',
    gauge     : '<path d="m12 14 4-4"/><path d="M3.34 19a10 10 0 1 1 17.32 0"/>',
    wallet    : '<path d="M21 12V7H5a2 2 0 0 1 0-4h14v4"/><path d="M3 5v14a2 2 0 0 0 2 2h16v-5"/><path d="M18 12a2 2 0 0 0 0 4h4v-4Z"/>',
    settings  : '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>',
    arrowUp   : '<path d="M7 17 17 7"/><path d="M8 7h9v9"/>',
    arrowDown : '<path d="m7 7 10 10"/><path d="M17 8v9H8"/>',
    external  : '<path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>',
    zap       : '<path d="M13 2 3 14h7l-1 8 11-13h-7l1-7z"/>',
  };
  return `<svg class="${cls}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">${paths[name] || ''}</svg>`;
};

/** 卡片容器 */
function Card(spanCls = 'col-span-12', inner = '') {
  return el(`
    <section class="card rise rounded-xl border border-zinc-800 bg-zinc-900/90 p-5 shadow-2xl shadow-black/10 ${spanCls}">
      ${inner}
    </section>`);
}

/** 卡片头部 */
function CardHeader({ title, desc, action = '' }) {
  return `
    <div class="mb-4 flex items-start justify-between gap-4">
      <div>
        <h3 class="text-sm font-semibold">${title}</h3>
        ${desc ? `<p class="mt-0.5 text-xs text-zinc-400">${desc}</p>` : ''}
      </div>
      ${action}
    </div>`;
}

/** 大数字统计卡 */
function StatCard({ spanCls, label, value, unit = '', delta = 0, deltaLabel = '较昨日', icon, spark }) {
  const up = delta >= 0;
  const deltaColor = up ? 'text-emerald-400' : 'text-amber-400';
  const deltaIcon = up ? Icon('arrowUp', 'h-3 w-3') : Icon('arrowDown', 'h-3 w-3');
  return el(`
    <section class="card rise rounded-xl border border-zinc-800 bg-zinc-900/90 p-5 shadow-2xl shadow-black/10 ${spanCls}">
      <div class="flex items-center justify-between">
        <span class="text-xs font-medium text-zinc-400">${label}</span>
        ${icon ? `<span class="grid h-8 w-8 place-items-center rounded-lg bg-cyan-400/10 text-cyan-400">${Icon(icon, 'h-4 w-4')}</span>` : ''}
      </div>
      <div class="mt-3 flex items-baseline gap-1.5">
        <span class="stat-value text-[28px] font-extrabold leading-none">${value}</span>
        ${unit ? `<span class="text-sm font-medium text-zinc-400">${unit}</span>` : ''}
      </div>
      <div class="mt-3 flex items-center gap-3">
        <span class="inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] font-semibold ${deltaColor} ${up ? 'bg-emerald-400/10' : 'bg-amber-400/10'}">
          ${deltaIcon}${Math.abs(delta)}%
        </span>
        <span class="text-[11px] text-zinc-500">${deltaLabel}</span>
      </div>
      ${spark ? `<div class="mt-3 -mb-1">${spark}</div>` : ''}
    </section>`);
}

/** 状态徽标 */
function Badge(text, tone = 'neutral') {
  const tones = {
    success: 'bg-emerald-400/10 text-emerald-400 border-emerald-400/20',
    warning: 'bg-amber-400/10 text-amber-400 border-amber-400/20',
    error  : 'bg-rose-400/10 text-rose-400 border-rose-400/20',
    cyan   : 'bg-cyan-400/10 text-cyan-400 border-cyan-400/20',
    neutral: 'bg-zinc-400/10 text-zinc-400 border-zinc-400/20',
  };
  return `<span class="inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-[11px] font-medium ${tones[tone]}">${text}</span>`;
}

/** 进度条 */
function ProgressBar({ value, warn = false, label, right }) {
  return `
    <div>
      <div class="mb-1.5 flex items-center justify-between text-xs">
        <span class="text-zinc-400">${label}</span>
        <span class="font-mono font-medium text-zinc-300">${right ?? value + '%'}</span>
      </div>
      <div class="h-1.5 overflow-hidden rounded-full bg-zinc-800">
        <div class="progress-fill ${warn ? 'warn' : ''} h-full rounded-full" style="width:0%" data-width="${Math.min(value, 100)}"></div>
      </div>
    </div>`;
}

/** SVG 迷你折线（sparkline） */
function Sparkline(points, { w = 220, h = 40, color = '#22d3ee' } = {}) {
  const max = Math.max(...points), min = Math.min(...points);
  const norm = (v) => h - 4 - ((v - min) / (max - min || 1)) * (h - 10);
  const step = w / (points.length - 1);
  const d = points.map((v, i) => `${i === 0 ? 'M' : 'L'}${(i * step).toFixed(1)},${norm(v).toFixed(1)}`).join(' ');
  return `
    <svg width="${w}" height="${h}" viewBox="0 0 ${w} ${h}" class="w-full">
      <defs>
        <linearGradient id="sg" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color="${color}" stop-opacity=".25"/>
          <stop offset="100%" stop-color="${color}" stop-opacity="0"/>
        </linearGradient>
      </defs>
      <path d="${d} L${w},${h} L0,${h} Z" fill="url(#sg)"/>
      <path d="${d}" fill="none" stroke="${color}" stroke-width="1.8" stroke-linecap="round"/>
      <circle cx="${w}" cy="${norm(points[points.length - 1])}" r="2.5" fill="${color}"/>
    </svg>`;
}

/** SVG 柱状趋势图（带 tooltip） */
function BarChart(days, { height = 180 } = {}) {
  const max = Math.max(1, ...days.map((d) => d.requests));
  return el(`
    <div class="relative flex items-end gap-[6px]" style="height:${height}px">
      ${days.map((d, i) => {
        const h = Math.round((d.requests / max) * (height - 34));
        const today = i === days.length - 1;
        return `
          <div class="bar-col group relative flex flex-1 flex-col items-center justify-end self-stretch">
            <div class="chart-tip z-10 rounded-lg border border-zinc-700 bg-zinc-800 px-2.5 py-1.5 text-[11px] shadow-xl">
              <div class="font-medium text-zinc-200">${d.date}</div>
              <div class="mt-0.5 font-mono text-cyan-400">${d.requests.toLocaleString()} 次 · $${d.cost.toFixed(2)}</div>
            </div>
            <div class="w-full max-w-[26px] rounded-t-md transition-all duration-200 ${today
              ? 'bg-gradient-to-t from-cyan-500 to-cyan-300 shadow-[0_0_18px_-2px_rgba(34,211,238,.5)]'
              : 'bg-zinc-700/80 group-hover:bg-cyan-400/60'}"
              style="height:${h}px"></div>
            <div class="absolute -bottom-6 text-[10px] ${today ? 'font-semibold text-cyan-400' : 'text-zinc-500'}">${d.label}</div>
          </div>`;
      }).join('')}
    </div>`);
}

/** 环形占比图 */
function DonutChart(items, { size = 168, thickness = 20 } = {}) {
  const total = items.reduce((s, i) => s + i.value, 0);
  const palette = ['#22d3ee', '#3b82f6', '#a78bfa', '#f59e0b', '#34d399', '#f472b6'];
  let acc = 0;
  const segs = items.map((it, i) => {
    const from = (acc / total) * 360; acc += it.value;
    const to = (acc / total) * 360;
    return `${palette[i % palette.length]} ${from}deg ${to}deg`;
  }).join(',');
  return el(`
    <div class="flex items-center gap-6">
      <div class="relative shrink-0" style="width:${size}px;height:${size}px">
        <div class="h-full w-full rounded-full" style="background:conic-gradient(${segs})"></div>
        <div class="absolute inset-0 grid place-items-center rounded-full"
             style="inset:${thickness / 2}px;background:var(--donut-hole, #18181b)"></div>
        <div class="absolute inset-0 grid place-items-center text-center">
          <div>
            <div class="stat-value text-xl font-extrabold">${total.toLocaleString()}</div>
            <div class="text-[10px] text-zinc-400">万 tokens</div>
          </div>
        </div>
      </div>
      <ul class="flex-1 space-y-2.5">
        ${items.map((it, i) => `
          <li class="flex items-center gap-2 text-xs">
            <span class="h-2.5 w-2.5 rounded-sm" style="background:${palette[i % palette.length]}"></span>
            <span class="flex-1 truncate text-zinc-400">${it.name}</span>
            <span class="font-mono font-medium">${((it.value / total) * 100).toFixed(1)}%</span>
          </li>`).join('')}
      </ul>
    </div>`);
}
