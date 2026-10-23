const byId = id => document.getElementById(id);
const result = text => { byId('action-result').textContent = text; };
const attach = (id, action) => byId(id)?.addEventListener('click',action);
attach('load-products',async () => {
  result('Loading products…');
  try { const response = await fetch('/api/products'); const data = await response.json(); result(`Received ${data.products.length} products. Inspect /api/products in Network.`); }
  catch (error) { result(`Request failed: ${error.message}`); }
});
attach('slow-request',async () => { const start=performance.now(); await fetch('/api/slow'); result(`Slow request completed in ${Math.round(performance.now()-start)} ms.`); });
attach('trigger-error',() => { result('Triggered an expected demo error. Inspect Console and subsequent reporting.'); setTimeout(()=>{throw new Error('Rumable demo: user-triggered test error');},0); });
attach('confirm-order',() => { result('Demo order confirmed. This is a simulated interaction without a transaction.'); });
document.querySelectorAll('.add-cart').forEach(button => button.addEventListener('click',()=>result(`Added ${button.dataset.product} to the demo list.`)));
function time(raw) { return new Date(raw).toLocaleTimeString('en-GB',{hour12:false}); }
async function refresh() {
  const count = document.querySelectorAll('script[data-rumable]').length;
  byId('injection-state').textContent = count === 1 ? 'Injected · once' : `${count} injection tags`;
  byId('injection-detail').textContent = document.querySelector('script[data-rumable]')?.dataset.rumable || 'No injection detected';
  byId('sdk-state').textContent = window.DATAFLUX_RUM?.getInternalContext?.() ? 'Running' : window.DATAFLUX_RUM ? 'SDK loaded' : 'Waiting for SDK';
  try {
    const response = await fetch('/__demo__/stats');
    const data = await response.json();
    byId('request-count').textContent = `${data.total} requests`;
    byId('last-request').textContent = data.requests[0] ? `Latest at ${time(data.requests[0].at)}` : 'Waiting for browser batches';
    const kinds = byId('event-types'); kinds.replaceChildren();
    for (const [name,count] of Object.entries(data.types)) { const tag=document.createElement('span'); tag.textContent=`${name} ${count}`; kinds.append(tag); }
    const list = byId('requests'); list.replaceChildren();
    for (const entry of data.requests.slice(0,8)) {
      const row=document.createElement('div'); row.className='request';
      const title=document.createElement('strong'); title.textContent=`POST /v1/write/rum`;
      const detail=document.createElement('small'); detail.textContent=`${time(entry.at)} · ${entry.bytes.toLocaleString()} bytes · ${Object.entries(entry.types).map(([k,n])=>`${k} ×${n}`).join(', ') || 'Request body received'}`;
      const badge=document.createElement('span'); badge.className='response-code'; badge.textContent='204';
      row.append(title,detail,badge); list.append(row);
    }
    if (!data.requests.length) { const empty=document.createElement('div'); empty.className='empty'; empty.textContent='No reports yet. Interact with the page and wait a few seconds.'; list.append(empty); }
  } catch { byId('last-request').textContent='Receipts temporarily unavailable'; }
}
refresh(); setInterval(()=>{if(!document.hidden)refresh();},5000);
