// Office-local calendar dates are plain "YYYY-MM-DD" strings. Arithmetic is
// done in UTC so the browser's own timezone and DST never shift a date.

export function addDays(iso, n) {
  const d = new Date(`${iso}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + n);
  return d.toISOString().slice(0, 10);
}

export function daysBetween(a, b) {
  return Math.round((new Date(`${b}T00:00:00Z`) - new Date(`${a}T00:00:00Z`)) / 86400000);
}

export function formatShort(iso) {
  // "Thu 13 Mar", as in the reference design
  return new Date(`${iso}T00:00:00Z`)
    .toLocaleDateString('en-GB', { weekday: 'short', day: 'numeric', month: 'short', timeZone: 'UTC' })
    .replace(',', '');
}

export function formatLong(iso) {
  return new Date(`${iso}T00:00:00Z`).toLocaleDateString('en-GB', {
    weekday: 'long', day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC',
  });
}

export function weekday(iso) {
  return new Date(`${iso}T00:00:00Z`).getUTCDay(); // 0 = Sunday
}

export const hh = (h) => `${String(h).padStart(2, '0')}:00`;

export function hourRange(from, to) {
  const out = [];
  for (let h = from; h <= to; h++) out.push(h);
  return out;
}
