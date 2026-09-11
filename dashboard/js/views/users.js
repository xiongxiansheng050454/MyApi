/* 视图：用户与 Key */
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
    const node = document.getElementById('keys-body');
    if (node) node.innerHTML = keysTableHTML(data?.list || []);
  } catch (err) {
    const node = document.getElementById('keys-body');
    if (node) node.innerHTML = `<div class="py-10 text-center text-xs text-rose-400">加载失败：${err.message}</div>`;
  }
}
