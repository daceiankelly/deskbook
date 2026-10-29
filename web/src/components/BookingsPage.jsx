import { useCallback, useEffect, useState } from 'react';
import { api } from '../api.js';
import { rules } from '../pyodide/bridge.js';
import { addDays, daysBetween, formatLong, formatShort, hh, hourRange } from '../lib/dates.js';

function EditRow({ booking, config, desks, onDone }) {
  const s = config.settings;
  const [form, setForm] = useState({ date: booking.date, startHour: booking.startHour, endHour: booking.endHour });
  const [errors, setErrors] = useState([]);
  const [saving, setSaving] = useState(false);
  const dates = Array.from({ length: daysBetween(config.today, config.lastDate) + 1 }, (_, i) => addDays(config.today, i));

  const save = async () => {
    const request = { deskId: booking.deskId, ...form };
    setSaving(true);
    try {
      // Instant feedback from Python against the latest bookings, then the server decides.
      const { bookings } = await api.bookings(form.date, form.date);
      const errs = await rules.checkBooking({
        request, settings: s, today: config.today, nowHour: config.nowHour, desks,
        bookings, ignoreBookingId: booking.id, employeeId: booking.employeeId,
      });
      if (errs.length) {
        setErrors(errs);
        return;
      }
      await api.changeBooking(booking.id, request);
      onDone(true);
    } catch (e) {
      setErrors([{ message: e.message }]);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="mt-3 rounded-md bg-slate-50 p-3">
      <div className="grid grid-cols-3 gap-2">
        <select className="input" value={form.date} onChange={(e) => setForm({ ...form, date: e.target.value })} aria-label="Date">
          {dates.map((d) => (
            <option key={d} value={d}>{formatShort(d)}</option>
          ))}
        </select>
        <select className="input" value={form.startHour} aria-label="Start"
          onChange={(e) => { const v = Number(e.target.value); setForm({ ...form, startHour: v, endHour: Math.max(form.endHour, v + 1) }); }}>
          {hourRange(s.openHour, s.closeHour - 1).map((h) => <option key={h} value={h}>{hh(h)}</option>)}
        </select>
        <select className="input" value={form.endHour} aria-label="End" onChange={(e) => setForm({ ...form, endHour: Number(e.target.value) })}>
          {hourRange(form.startHour + 1, s.closeHour).map((h) => <option key={h} value={h}>{hh(h)}</option>)}
        </select>
      </div>
      {errors.length > 0 && (
        <ul className="mt-2 text-sm text-red-700" role="alert">
          {errors.map((e, i) => <li key={i}>• {e.message}</li>)}
        </ul>
      )}
      <div className="mt-3 flex gap-2">
        <button className="btn-primary px-3 py-1.5" onClick={save} disabled={saving}>{saving ? 'Saving…' : 'Save change'}</button>
        <button className="btn-ghost px-3 py-1.5" onClick={() => onDone(false)}>Close</button>
      </div>
    </div>
  );
}

export default function BookingsPage({ config, user, desks, onChanged, onShowOnMap }) {
  const [lists, setLists] = useState(null);
  const [editing, setEditing] = useState(null);
  const [error, setError] = useState(null);

  const load = useCallback(async () => {
    try {
      const { bookings } = await api.myBookings();
      setLists(await rules.myBookings({ bookings, userId: user.id, today: config.today, nowHour: config.nowHour }));
      setError(null);
    } catch (e) {
      setError(e.message);
    }
  }, [user.id, config.today, config.nowHour]);

  useEffect(() => {
    load();
  }, [load]);

  const cancel = async (b) => {
    if (!window.confirm(`Cancel desk ${b.deskLabel} on ${formatLong(b.date)} ${hh(b.startHour)}–${hh(b.endHour)}?`)) return;
    try {
      await api.cancelBooking(b.id);
      await load();
      onChanged();
    } catch (e) {
      setError(e.message);
    }
  };

  return (
    <div className="h-full overflow-y-auto bg-slate-50/50 px-10 py-8">
      <h1 className="text-2xl font-bold">My bookings</h1>
      {error && <div className="mt-4 rounded-md bg-red-50 p-3 text-sm text-red-800" role="alert">{error}</div>}
      {!lists ? (
        <p className="mt-6 text-ink-muted">Loading…</p>
      ) : (
        <div className="mt-6 grid max-w-5xl gap-8 lg:grid-cols-2">
          <section>
            <h2 className="label">Upcoming ({lists.upcoming.length})</h2>
            {lists.upcoming.length === 0 && <p className="card p-5 text-sm text-ink-muted">No upcoming bookings. Pick a desk on the map.</p>}
            <ul className="space-y-3">
              {lists.upcoming.map((b) => (
                <li key={b.id} className="card p-4">
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <div className="text-sm text-ink-muted">{formatLong(b.date)}</div>
                      <div className="text-lg font-semibold">
                        {hh(b.startHour)}–{hh(b.endHour)} · Desk {b.deskLabel}
                      </div>
                    </div>
                    <span className="mt-1 h-3 w-3 shrink-0 rounded-full bg-desk-booked" />
                  </div>
                  <div className="mt-3 flex flex-wrap gap-2">
                    <button className="btn-ghost px-3 py-1.5" onClick={() => onShowOnMap(b)}>Show on map</button>
                    <button className="btn-ghost px-3 py-1.5" onClick={() => setEditing(editing === b.id ? null : b.id)}>Change</button>
                    <button className="btn-danger px-3 py-1.5" onClick={() => cancel(b)}>Cancel booking</button>
                  </div>
                  {editing === b.id && (
                    <EditRow
                      booking={b}
                      config={config}
                      desks={desks}
                      onDone={async (changed) => {
                        setEditing(null);
                        if (changed) {
                          await load();
                          onChanged();
                        }
                      }}
                    />
                  )}
                </li>
              ))}
            </ul>
          </section>
          <section>
            <h2 className="label">Past & cancelled ({lists.past.length})</h2>
            {lists.past.length === 0 && <p className="card p-5 text-sm text-ink-muted">Nothing yet.</p>}
            <ul className="card divide-y divide-slate-100">
              {lists.past.map((b) => (
                <li key={b.id} className="flex items-center justify-between px-4 py-3 text-sm">
                  <span>
                    <span className="font-medium">{formatShort(b.date)}</span> {hh(b.startHour)}–{hh(b.endHour)} · {b.deskLabel}
                  </span>
                  <span className={`text-xs font-semibold ${b.status === 'cancelled' ? 'text-red-600' : 'text-ink-muted'}`}>
                    {b.status === 'cancelled' ? 'Cancelled' : 'Completed'}
                  </span>
                </li>
              ))}
            </ul>
          </section>
        </div>
      )}
    </div>
  );
}
