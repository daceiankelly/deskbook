import { formatLong, hh } from '../lib/dates.js';
import { deskColor } from './FloorPlan.jsx';
import { XIcon } from './icons.jsx';

const STATE_LABEL = {
  available: 'Available',
  selected: 'Selected',
  booked: 'Booked',
  unavailable: 'Unavailable',
};

/** Right-hand drawer: desk details, rule feedback from Python, confirm/cancel. */
export default function BookingPanel({
  desk,
  deskState,
  date,
  startHour,
  endHour,
  deskBookings,
  freeSlots,
  errors,
  submitting,
  message,
  onConfirm,
  onCancelBooking,
  onPickSlot,
  onClose,
}) {
  const state = deskState?.state || 'unavailable';
  const bookable = state === 'available' || state === 'selected';
  // Rule feedback only matters while the desk can be booked; for a booked desk
  // (including your own booking) the status line already says why.
  const shownErrors = bookable ? errors : [];
  const canBook = bookable && errors.length === 0 && !submitting;

  return (
    <aside className="z-20 flex h-full w-[360px] shrink-0 flex-col border-l border-slate-100 bg-white shadow-[-4px_0_16px_rgba(15,23,42,0.08)]" aria-label="Booking details">
      <header className="flex items-start justify-between border-b border-slate-100 p-5">
        <div>
          <div className="text-xs font-semibold uppercase tracking-wide text-ink-muted">Desk</div>
          <h2 className="text-xl font-bold">{desk.label}</h2>
          {desk.zone && <div className="text-sm text-ink-muted">Zone {desk.zone}</div>}
        </div>
        <button onClick={onClose} className="rounded p-1 hover:bg-slate-100" aria-label="Close panel">
          <XIcon />
        </button>
      </header>

      <div className="flex-1 space-y-5 overflow-y-auto p-5">
        <div className="flex items-center gap-2">
          <span className="h-3 w-3 rounded-full" style={{ background: deskColor(deskState?.mine ? 'booked' : state) }} />
          <span className="text-sm font-semibold">{deskState?.mine ? 'Booked by you' : STATE_LABEL[state]}</span>
        </div>

        <section className="rounded-lg bg-slate-50 p-4">
          <div className="text-sm font-semibold">{formatLong(date)}</div>
          <div className="text-2xl font-bold tracking-tight">
            {hh(startHour)} – {hh(endHour)}
          </div>
          <div className="text-xs text-ink-muted">{endHour - startHour} hour{endHour - startHour === 1 ? '' : 's'} · office time</div>
        </section>

        {shownErrors.length > 0 && (
          <ul className="space-y-1 rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900" role="alert">
            {shownErrors.map((e, i) => (
              <li key={i}>• {e.message}</li>
            ))}
          </ul>
        )}

        {message && (
          <div role="status" className={`rounded-md p-3 text-sm ${message.ok ? 'bg-green-50 text-green-800' : 'bg-red-50 text-red-800'}`}>
            {message.text}
          </div>
        )}

        {deskBookings.length > 0 && (
          <section>
            <h3 className="label">Bookings on this desk</h3>
            <ul className="divide-y divide-slate-100 rounded-md border border-slate-100">
              {deskBookings.map((b) => (
                <li key={b.id} className="flex items-center justify-between px-3 py-2 text-sm">
                  <span>
                    <span className="font-medium">{hh(b.startHour)}–{hh(b.endHour)}</span>{' '}
                    <span className="text-ink-muted">{b.mine ? 'You' : b.employeeName}</span>
                  </span>
                  {b.mine && (
                    <button className="text-xs font-semibold text-red-600 hover:underline" onClick={() => onCancelBooking(b)} disabled={submitting}>
                      Cancel
                    </button>
                  )}
                </li>
              ))}
            </ul>
          </section>
        )}

        {freeSlots.length > 0 && state !== 'unavailable' && (
          <section>
            <h3 className="label">Free on this desk</h3>
            <div className="flex flex-wrap gap-2">
              {freeSlots.map((s) => (
                <button
                  key={s.startHour}
                  onClick={() => onPickSlot(s)}
                  className="rounded-full border border-desk-available px-3 py-1 text-xs font-semibold text-green-700 hover:bg-green-50"
                >
                  {hh(s.startHour)}–{hh(s.endHour)}
                </button>
              ))}
            </div>
          </section>
        )}
      </div>

      <footer className="border-t border-slate-100 p-5">
        <button className="btn-primary w-full py-3" disabled={!canBook} onClick={onConfirm}>
          {submitting ? 'Booking…' : `Confirm booking ${desk.label}`}
        </button>
        <p className="mt-2 text-center text-xs text-ink-muted">The server re-checks availability when you confirm.</p>
      </footer>
    </aside>
  );
}
