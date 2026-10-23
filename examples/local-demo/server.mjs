import http from 'node:http';
import {readFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import {gunzipSync, inflateSync} from 'node:zlib';
const publicDir = fileURLToPath(new URL('./public/', import.meta.url));
const requests = [];
const startedAt = new Date().toISOString();
let total = 0;
const types = {};
const json = (res, status, body) => { res.writeHead(status, {'Content-Type':'application/json; charset=utf-8','Cache-Control':'no-store'}); res.end(JSON.stringify(body)); };
const files = new Map([
  ['/', ['index.html','text/html; charset=utf-8']],
  ['/checkout/', ['checkout.html','text/html; charset=utf-8']],
  ['/app.js', ['app.js','application/javascript; charset=utf-8']],
  ['/styles.css', ['styles.css','text/css; charset=utf-8']],
]);
const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, 'http://localhost');
  if (url.pathname.startsWith('/v1/')) {
    res.setHeader('Access-Control-Allow-Origin','*');
    res.setHeader('Access-Control-Allow-Methods','POST, GET, OPTIONS');
    res.setHeader('Access-Control-Allow-Headers','Content-Type');
    res.setHeader('Cache-Control','no-store');
    if (req.method === 'OPTIONS') { res.writeHead(204); res.end(); return; }
    if (req.method !== 'POST' || url.pathname !== '/v1/write/rum') { json(res,404,{error:'Only local RUM POST ingestion is simulated'}); return; }
    const chunks = []; let length = 0;
    for await (const chunk of req) {
      length += chunk.length;
      if (length > 2*1024*1024) { res.writeHead(413); res.end(); req.resume(); return; }
      chunks.push(chunk);
    }
    let body = Buffer.concat(chunks);
    try {
      if (req.headers['content-encoding'] === 'gzip') body = gunzipSync(body);
      if (req.headers['content-encoding'] === 'deflate') body = inflateSync(body);
    } catch { json(res,400,{error:'Invalid compressed payload'}); return; }
    const seen = {};
    const count = kind => { seen[kind] = (seen[kind] || 0) + 1; types[kind] = (types[kind] || 0) + 1; };
    for (const line of body.toString('utf8').split('\n')) {
      try {
        const entry = JSON.parse(line);
        for (const event of Array.isArray(entry) ? entry : [entry]) {
          count(event.measurement || event.type || event.event?.type || 'other');
        }
      } catch {
        // The SDK defaults to line protocol: measurement,tags fields timestamp.
        const match = /^(view|resource|action|error|long_task|telemetry|logger),/.exec(line);
        if (match) count(match[1]);
      }
    }
    const item = {sequence:++total, at:new Date().toISOString(), method:req.method, path:url.pathname, bytes:body.length, types:seen};
    requests.unshift(item); requests.splice(40);
    console.log(JSON.stringify({event:'local-intake', ...item}));
    res.writeHead(204); res.end(); return;
  }
  if (url.pathname === '/__demo__/stats') { json(res,200,{startedAt, total, types, requests, simulation:true}); return; }
  if (url.pathname === '/api/products') {
    await new Promise(resolve => setTimeout(resolve,180));
    json(res,200,{products:[{id:'desk-01',name:'Minimal desk lamp',price:129},{id:'cup-02',name:'Travel coffee cup',price:69}], server:'Linux Node web service', at:new Date().toISOString()}); return;
  }
  if (url.pathname === '/api/slow') {
    await new Promise(resolve => setTimeout(resolve,900));
    json(res,200,{ok:true,delayMs:900}); return;
  }
  const file = files.get(url.pathname);
  if (!file) { json(res,404,{error:'Not found'}); return; }
  try {
    const data = await readFile(publicDir+file[0]);
    res.writeHead(200,{'Content-Type':file[1],'Cache-Control':'no-store'});
    res.end(req.method === 'HEAD' ? undefined : data);
  } catch { json(res,500,{error:'Fixture file could not be read'}); }
});
server.listen(3000,'0.0.0.0',()=>console.log('Demo web service and local intake listening on 3000'));
