// Copies the Pyodide runtime from node_modules into public/pyodide so it is
// served from the office server. Nothing is fetched from a CDN at runtime.
import { cpSync, existsSync, mkdirSync, readdirSync, rmSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const src = join(root, 'node_modules', 'pyodide');
const dest = join(root, 'public', 'pyodide');

const REQUIRED = ['pyodide.mjs', 'pyodide.asm.js', 'pyodide.asm.wasm', 'python_stdlib.zip', 'pyodide-lock.json'];
const SKIP = /(\.d\.ts|\.map|package\.json|README.*|LICENSE.*)$/i;

if (!existsSync(src)) {
  console.error('pyodide is not installed; run `npm install` first');
  process.exit(1);
}
rmSync(dest, { recursive: true, force: true });
mkdirSync(dest, { recursive: true });
for (const f of readdirSync(src)) {
  if (!SKIP.test(f)) cpSync(join(src, f), join(dest, f), { recursive: true });
}
const missing = REQUIRED.filter((f) => !existsSync(join(dest, f)));
if (missing.length) {
  console.error('Pyodide files missing after copy:', missing.join(', '));
  process.exit(1);
}
console.log(`Pyodide runtime copied to ${dest}`);
