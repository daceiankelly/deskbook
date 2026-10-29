// Static offline check of the production build (dist/):
//  1. index.html and CSS must not load anything from another host.
//  2. Every required Pyodide runtime file must be present locally.
//  3. Our own JS bundle must not contain fetch/import calls to absolute http(s) URLs.
// The Go server's CSP (connect-src 'self') additionally makes the browser
// refuse any non-LAN request at runtime. A manual check with the network
// cable unplugged is described in docs/testing.md.
import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, extname } from 'node:path';
import { fileURLToPath } from 'node:url';

const dist = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const problems = [];

function walk(dir) {
  return readdirSync(dir).flatMap((f) => {
    const p = join(dir, f);
    return statSync(p).isDirectory() ? walk(p) : [p];
  });
}

if (!existsSync(join(dist, 'index.html'))) {
  console.error('dist/index.html not found: run `npm run build` first');
  process.exit(1);
}

const external = /(?:src|href)\s*=\s*["'](?:https?:)?\/\//gi;
const html = readFileSync(join(dist, 'index.html'), 'utf8');
if (external.test(html)) problems.push('index.html references an external host');

for (const file of walk(join(dist, 'assets'))) {
  const text = readFileSync(file, 'utf8');
  if (extname(file) === '.css' && /url\(\s*["']?(?:https?:)?\/\//i.test(text)) {
    problems.push(`${file}: CSS loads an external URL`);
  }
  if (extname(file) === '.js' && /(?:fetch|import)\s*\(\s*["'`]https?:\/\//i.test(text)) {
    problems.push(`${file}: JS fetches/imports an absolute external URL`);
  }
}

for (const f of ['pyodide.mjs', 'pyodide.asm.js', 'pyodide.asm.wasm', 'python_stdlib.zip', 'pyodide-lock.json']) {
  if (!existsSync(join(dist, 'pyodide', f))) problems.push(`missing local Pyodide file: pyodide/${f}`);
}

if (problems.length) {
  console.error('Offline check FAILED:\n  ' + problems.join('\n  '));
  process.exit(1);
}
console.log('Offline check passed: no external hosts referenced; Pyodide runtime bundled locally.');
