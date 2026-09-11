/* 视图：渠道管理 */
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
