/* 视图：计费定价 */
async function renderPricingView() {
  const bento = document.getElementById('bento');
  const card = Card('col-span-12', `
    ${CardHeader({ title: '渠道×模型定价', desc: 'model_pricing · 单位：美元 / 1M tokens' })}
    <div id="pricing-body" class="py-10 text-center text-xs text-zinc-500">加载中…</div>`);
  bento.append(card);
  try {
    const data = await adminGet('/pricing', { page: 1, page_size: 100 });
    const node = document.getElementById('pricing-body');
    if (node) node.innerHTML = pricingTableHTML(data?.list || []);
  } catch (err) {
    const node = document.getElementById('pricing-body');
    if (node) node.innerHTML = `<div class="py-10 text-center text-xs text-rose-400">加载失败：${err.message}</div>`;
  }
}
