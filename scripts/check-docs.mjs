import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const forbidden = new RegExp(['g' + 'uance', 'cloud' + 'care', '\u89c2\u6d4b\u4e91'].join('|'), 'i');
const han = /\p{Script=Han}/u;
const files = ['README.md', 'USER-GUIDE.md', 'CHANGELOG.md', 'web/index.html', 'web/i18n.mjs', 'scripts/release.go', 'scripts/ci/publish-oss.mjs'];
function collect(directory, accept) {
  for (const item of fs.readdirSync(path.join(root, directory), {withFileTypes:true})) {
    const name = path.posix.join(directory, item.name);
    if (item.isDirectory()) collect(name, accept);
    else if (accept(name)) files.push(name);
  }
}
collect('docs', name => name.endsWith('.md'));
collect('examples', name => /\.(?:md|toml|html|js)$/.test(name));
// Vendored SDK files, machine source metadata, required license notices, and
// internal authoring skills are outside the public prose surface.
const errors = [];
const withoutFences = text => text.replace(/^(```|~~~)[^\n]*\n[\s\S]*?^\1[ \t]*$/gm, '');
function anchors(text) {
  const used = new Map(), result = new Set();
  for (const line of withoutFences(text).split('\n')) {
    const heading = /^#{1,6}\s+(.+?)(?:\s+#+)?$/.exec(line);
    if (!heading) continue;
    const base = heading[1].toLowerCase().replace(/<[^>]*>/g, '').replace(/[^\p{L}\p{N}_\-\s]/gu, '').replace(/\s/g, '-');
    const seen = used.get(base) || 0;
    used.set(base, seen + 1);
    result.add(base + (seen ? '-' + seen : ''));
  }
  for (const match of text.matchAll(/<(?:a|h[1-6])\b[^>]*(?:id|name)=["']([^"']+)["']/g)) result.add(match[1]);
  return result;
}
for (const name of files) {
  const text = fs.readFileSync(path.join(root, name), 'utf8');
  for (const [offset, line] of text.split('\n').entries()) {
    if (forbidden.test(line)) errors.push(name + ':' + (offset + 1) + ': prohibited product wording or URL');
    if (han.test(line)) errors.push(name + ':' + (offset + 1) + ': public documentation must be English');
  }
  if (!name.endsWith('.md')) continue;
  for (const match of withoutFences(text).matchAll(/!?\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)/g)) {
    const reference = match[1].replace(/^<|>$/g, '');
    if (/^[a-z][a-z0-9+.-]*:/i.test(reference) || reference.startsWith('//')) continue;
    let target, fragment;
    try {
      const hash = reference.indexOf('#');
      const targetPart = hash < 0 ? reference : reference.slice(0, hash);
      target = path.resolve(root, path.dirname(name), decodeURIComponent(targetPart || path.basename(name)));
      fragment = hash < 0 ? '' : decodeURIComponent(reference.slice(hash + 1));
    } catch {
      errors.push(name + ': invalid relative URL ' + reference);
      continue;
    }
    if (!target.startsWith(root + path.sep) || !fs.existsSync(target)) {
      errors.push(name + ': missing local link ' + reference);
    } else if (fragment && target.endsWith('.md') && !anchors(fs.readFileSync(target, 'utf8')).has(fragment)) {
      errors.push(name + ': missing anchor ' + reference);
    }
  }
}
if (errors.length) {
  process.stderr.write(errors.join('\n') + '\n');
  process.exitCode = 1;
} else {
  process.stdout.write('English documentation, product wording, and local links passed (' + files.length + ' files).\n');
}
