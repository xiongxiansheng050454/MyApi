/* 视图：限流规则 */
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
