/* 视图：请求日志 */
function renderLogsView() {
  const bento = document.getElementById('bento');
  const card = Card('col-span-12', `
    ${CardHeader({ title: '请求日志', desc: 'usage_logs · 最近记录（含成功/失败）' })}
    ${logsTableHTML(RECENT_LOGS)}`);
  bento.append(card);
}
