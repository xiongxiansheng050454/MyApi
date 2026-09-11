/* 视图：用量统计 */
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
