import { useMemo, useState } from 'react';
import { formatLong, hh } from '../lib/dates.js';
import { SearchIcon } from './icons.jsx';

/** Who is in the office on the selected date. */
export default function UsersPage({ colleagues, bookingsOnDate, date, onShowOnMap }) {
  const [q, setQ] = useState('');
  const rows = useMemo(() => {
    const s = q.trim().toLowerCase();
    return colleagues
      .filter((c) => !s || c.name.toLowerCase().includes(s))
      .map((c) => ({ ...c, bookings: bookingsOnDate.filter((b) => b.employeeId === c.id) }))
      .sort((a, b) => (b.bookings.length > 0) - (a.bookings.length > 0) || a.name.localeCompare(b.name));
  }, [q, colleagues, bookingsOnDate]);

  return (
    <div className="h-full overflow-y-auto px-10 py-8">
      <h1 className="text-2xl font-bold">Colleagues</h1>
      <p className="text-sm text-ink-muted">{formatLong(date)}</p>
      <div className="mt-4 flex h-11 max-w-md items-center gap-2 rounded-full bg-white px-4 shadow-card">
        <SearchIcon className="h-4 w-4" />
        <input className="w-full text-sm outline-none" placeholder="Filter by name" value={q} onChange={(e) => setQ(e.target.value)} />
      </div>
      <ul className="card mt-6 max-w-3xl divide-y divide-slate-100">
        {rows.map((c) => (
          <li key={c.id} className="flex items-center gap-4 px-4 py-3">
            <span className="flex h-9 w-9 items-center justify-center rounded-full bg-brand-pale text-xs font-bold text-brand">{c.initials}</span>
            <span className="flex-1">
              <span className="block font-medium">{c.name}</span>
              <span className="block text-xs text-ink-muted">
                {c.bookings.length ? c.bookings.map((b) => `Desk ${b.deskLabel} ${hh(b.startHour)}–${hh(b.endHour)}`).join(' · ') : 'Not in the office'}
              </span>
            </span>
            {c.bookings.length > 0 && (
              <button className="btn-ghost px-3 py-1.5" onClick={() => onShowOnMap(c)}>Show on map</button>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}
