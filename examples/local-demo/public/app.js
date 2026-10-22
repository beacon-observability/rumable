const byId = id => document.getElementById(id);
const result = text => { byId('action-result').textContent = text; };
const attach = (id, action) => byId(id)?.addEventListener('click',action);
attach('load-products',async () => {
  result('正在请求商品数据…');
  try { const response = await fetch('/api/products'); const data = await response.json(); result(`已收到 ${data.products.length} 件商品。Network 可查看 /api/products。`); }
  catch (error) { result(`请求失败：${error.message}`); }
});
attach('slow-request',async () => { const start=performance.now(); await fetch('/api/slow'); result(`慢请求完成，耗时 ${Math.round(performance.now()-start)} ms。`); });
attach('trigger-error',() => { result('已触发一条预期的演示错误。请查看 Console 和后续上报。'); setTimeout(()=>{throw new Error('Rumable demo: user-triggered test error');},0); });
attach('confirm-order',() => { result('演示订单已确认。这只是页面交互，没有真实交易。'); });
document.querySelectorAll('.add-cart').forEach(button => button.addEventListener('click',()=>result(`已将「${button.dataset.product}」加入演示清单。`)));
function time(raw) { return new Date(raw).toLocaleTimeString('zh-CN',{hour12:false}); }
async function refresh() {
  const count = document.querySelectorAll('script[data-rumable]').length;
  byId('injection-state').textContent = count === 1 ? '已注入 · 1 次' : `注入标签 ${count} 个`;
  byId('injection-detail').textContent = document.querySelector('script[data-rumable]')?.dataset.rumable || '未检测到注入';
  byId('sdk-state').textContent = window.DATAFLUX_RUM?.getInternalContext?.() ? '正在运行' : window.DATAFLUX_RUM ? 'SDK 已加载' : '等待加载';
  try {
    const response = await fetch('/__demo__/stats');
    const data = await response.json();
    byId('request-count').textContent = `${data.total} 次`;
    byId('last-request').textContent = data.requests[0] ? `最近一次 ${time(data.requests[0].at)}` : '等待浏览器批量发送';
    const kinds = byId('event-types'); kinds.replaceChildren();
    for (const [name,count] of Object.entries(data.types)) { const tag=document.createElement('span'); tag.textContent=`${name} ${count}`; kinds.append(tag); }
    const list = byId('requests'); list.replaceChildren();
    for (const entry of data.requests.slice(0,8)) {
      const row=document.createElement('div'); row.className='request';
      const title=document.createElement('strong'); title.textContent=`POST /v1/write/rum`;
      const detail=document.createElement('small'); detail.textContent=`${time(entry.at)} · ${entry.bytes.toLocaleString()} bytes · ${Object.entries(entry.types).map(([k,n])=>`${k} ×${n}`).join(', ') || '请求体已接收'}`;
      const badge=document.createElement('span'); badge.className='response-code'; badge.textContent='204';
      row.append(title,detail,badge); list.append(row);
    }
    if (!data.requests.length) { const empty=document.createElement('div'); empty.className='empty'; empty.textContent='还没有上报，请操作页面后等待几秒钟。'; list.append(empty); }
  } catch { byId('last-request').textContent='暂时无法读取接收记录'; }
}
refresh(); setInterval(()=>{if(!document.hidden)refresh();},5000);
