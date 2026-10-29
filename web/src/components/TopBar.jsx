import { useEffect, useMemo, useRef, useState } from 'react';
import { addDays, daysBetween, formatShort, hh, hourRange, weekday } from '../lib/dates.js';
import { CalendarIcon, ChevronLeft, ChevronRight, ClockIcon, FilterIcon, SearchIcon, XIcon } from './icons.jsx';

function useOutsideClose(open, setOpen) {
  const ref = useRef(null);
  useEffect(() => {
    if (!open) return undefined;
    const h = (e) => ref.current && !ref.current.contains(e.target) && setOpen(false);
    const k = (e) => e.key === 'Escape' && setOpen(false);
    document.addEventListener('pointerdown', h);
    document.addEventListener('keydown', k);
    return () => {
      document.removeEventListener('pointerdown', h);
      document.removeEventListener('keydown', k);
    };
  }, [open, setOpen]);
  return ref;
}

/** Rolling-window date selector: "Thu 13 Mar" pill with ✕ and ‹ › plus a month-free 60-day grid. */
export function DateControl({ date, today, lastDate, onChange }) {
  const [open, setOpen] = useState(false);
  const ref = useOutsideClose(open, setOpen);
  const days = useMemo(() => {
    const total = daysBetween(today, lastDate) + 1;
    const lead = (weekday(today) + 6) % 7; // Monday-first grid
    return [...Array(lead).fill(null), ...Array.from({ length: total }, (_, i) => addDays(today, i))];
  }, [today, lastDate]);

  return (
    <div ref={ref} className="relative">
      <div className="flex h-11 items-center gap-1 rounded-md border-2 border-brand bg-white pl-3 pr-1 shadow-card">
        <button className="flex items-center gap-2 pr-1 text-sm font-semibold" onClick={() => setOpen((o) => !o)} aria-haspopup="dialog">
          <CalendarIcon className="h-[18px] w-[18px]" />
          <span className="min-w-[78px] text-left">{formatShort(date)}</span>
        </button>
        <button
          className="flex h-6 w-6 items-center justify-center rounded-full bg-brand text-white disabled:opacity-30"
          onClick={() => onChange(today)}
          disabled={date === today}
          aria-label="Back to today"
        >
          <XIcon className="h-3.5 w-3.5" strokeWidth={3} />
        </button>
        <button className="ml-1 rounded p-1.5 hover:bg-slate-100 disabled:opacity-30" disabled={date <= today} onClick={() => onChange(addDays(date, -1))} aria-label="Previous day">
          <ChevronLeft className="h-4 w-4" strokeWidth={2.5} />
        </button>
        <span className="h-5 w-px bg-slate-200" />
        <button className="rounded p-1.5 hover:bg-slate-100 disabled:opacity-30" disabled={date >= lastDate} onClick={() => onChange(addDays(date, 1))} aria-label="Next day">
          <ChevronRight className="h-4 w-4" strokeWidth={2.5} />
        </button>
      </div>
      {open && (
        <div role="dialog" aria-label="Choose date" className="absolute left-0 top-14 z-30 w-[320px] rounded-lg bg-white p-3 shadow-card ring-1 ring-slate-100">
          <div className="mb-2 flex justify-between text-xs font-semibold text-ink-muted">
            <span>Bookable: {formatShort(today)} – {formatShort(lastDate)}</span>
          </div>
          <div className="grid grid-cols-7 gap-1 text-center text-[11px] font-semibold text-ink-muted">
            {['Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa', 'Su'].map((d) => (
              <span key={d}>{d}</span>
            ))}
          </div>
          <div className="mt-1 grid max-h-[260px] grid-cols-7 gap-1 overflow-y-auto">
            {days.map((d, i) =>
              d ? (
                <button
                  key={d}
                  onClick={() => {
                    onChange(d);
                    setOpen(false);
                  }}
                  className={`rounded py-1.5 text-sm ${
                    d === date ? 'bg-brand font-semibold text-white' : d === today ? 'font-semibold text-brand hover:bg-brand-pale' : 'hover:bg-slate-100'
                  }`}
                  title={formatShort(d)}
                >
                  {Number(d.slice(8))}
                  {d.slice(8) === '01' && <span className="block text-[9px] leading-none">{formatShort(d).split(' ')[2]}</span>}
                </button>
              ) : (
                <span key={`pad-${i}`} />
              ),
            )}
          </div>
        </div>
      )}
    </div>
  );
}

/** Round filter button (reference) opening hourly start/end selection. */
export function TimeFilter({ startHour, endHour, openHour, closeHour, minStart, onChange }) {
  const [open, setOpen] = useState(false);
  const ref = useOutsideClose(open, setOpen);
  return (
    <div ref={ref} className="relative flex items-center gap-2">
      <button className="flex h-11 w-11 items-center justify-center rounded-full bg-white shadow-card hover:bg-slate-50" onClick={() => setOpen((o) => !o)} aria-label="Choose time">
        <FilterIcon />
      </button>
      <button onClick={() => setOpen(true)} className="flex items-center gap-1.5 rounded-full bg-white px-3 py-1.5 text-sm font-medium shadow-card">
        <ClockIcon className="h-4 w-4 text-ink-muted" />
        {hh(startHour)}–{hh(endHour)}
      </button>
      {open && (
        <div className="absolute left-0 top-14 z-30 w-64 rounded-lg bg-white p-4 shadow-card ring-1 ring-slate-100">
          <label className="label" htmlFor="start-hour">Start time</label>
          <select
            id="start-hour"
            className="input mb-3"
            value={startHour}
            onChange={(e) => {
              const s = Number(e.target.value);
              onChange(s, Math.max(endHour, s + 1));
            }}
          >
            {hourRange(Math.max(openHour, minStart), closeHour - 1).map((h) => (
              <option key={h} value={h}>{hh(h)}</option>
            ))}
          </select>
          <label className="label" htmlFor="end-hour">End time</label>
          <select id="end-hour" className="input" value={endHour} onChange={(e) => onChange(startHour, Number(e.target.value))}>
            {hourRange(startHour + 1, closeHour).map((h) => (
              <option key={h} value={h}>{hh(h)}</option>
            ))}
          </select>
          <div className="mt-3 flex gap-2">
            <button className="btn-ghost flex-1 px-2 py-1 text-xs" onClick={() => onChange(Math.max(openHour, minStart), Math.min(closeHour, 13))} disabled={Math.max(openHour, minStart) >= 13}>Morning</button>
            <button className="btn-ghost flex-1 px-2 py-1 text-xs" onClick={() => onChange(Math.max(13, minStart), closeHour)} disabled={Math.max(13, minStart) >= closeHour}>Afternoon</button>
            <button className="btn-ghost flex-1 px-2 py-1 text-xs" onClick={() => onChange(Math.max(openHour, minStart), closeHour)}>All day</button>
          </div>
        </div>
      )}
    </div>
  );
}

export function LocationBadge({ office, floor, available }) {
  return (
    <div className="flex h-11 items-center gap-2 rounded-md bg-white px-3 text-sm shadow-card">
      <span className="font-semibold">{office}</span>
      <span className="text-ink-muted">/</span>
      <span className="text-ink-muted">{floor}</span>
      <span className="rounded bg-desk-available px-1.5 py-0.5 text-xs font-bold text-ink" title="Desks available for the selected time">
        {available ?? '…'}
      </span>
    </div>
  );
}

/** "Find a colleague": shows where a colleague sits on the selected date. */
export function ColleagueSearch({ colleagues, bookingsOnDate, onPick }) {
  const [q, setQ] = useState('');
  const [open, setOpen] = useState(false);
  const ref = useOutsideClose(open, setOpen);
  const results = useMemo(() => {
    const s = q.trim().toLowerCase();
    if (!s) return [];
    return colleagues
      .filter((c) => c.name.toLowerCase().includes(s))
      .slice(0, 8)
      .map((c) => ({ ...c, bookings: bookingsOnDate.filter((b) => b.employeeId === c.id) }));
  }, [q, colleagues, bookingsOnDate]);

  return (
    <div ref={ref} className="relative w-full max-w-[560px]">
      <div className="flex h-11 items-center gap-2 rounded-full bg-white px-4 shadow-card">
        <SearchIcon className="h-[18px] w-[18px] text-ink" />
        <input
          className="w-full bg-transparent text-sm outline-none placeholder:text-ink-muted"
          placeholder="Find a colleague"
          value={q}
          onChange={(e) => {
            setQ(e.target.value);
            setOpen(true);
          }}
          onFocus={() => setOpen(true)}
          aria-label="Find a colleague"
        />
        {q && (
          <button onClick={() => { setQ(''); onPick(null); }} aria-label="Clear search">
            <XIcon className="h-4 w-4 text-ink-muted" />
          </button>
        )}
      </div>
      {open && results.length > 0 && (
        <ul className="absolute left-0 right-0 top-12 z-30 max-h-80 overflow-y-auto rounded-lg bg-white py-1 shadow-card ring-1 ring-slate-100">
          {results.map((c) => (
            <li key={c.id}>
              <button
                className="flex w-full items-center gap-3 px-4 py-2 text-left text-sm hover:bg-slate-50"
                onClick={() => {
                  onPick(c);
                  setOpen(false);
                }}
              >
                <span className="flex h-8 w-8 items-center justify-center rounded-full bg-brand-pale text-xs font-bold text-brand">{c.initials}</span>
                <span className="flex-1">
                  <span className="block font-medium">{c.name}</span>
                  <span className="block text-xs text-ink-muted">
                    {c.bookings.length
                      ? c.bookings.map((b) => `${b.deskLabel} ${hh(b.startHour)}–${hh(b.endHour)}`).join(', ')
                      : 'No desk booked this day'}
                  </span>
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
