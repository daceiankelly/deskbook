import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { api, ApiError, setCsrfToken, setUnauthorizedHandler } from './api.js';
import { loadRules, rules } from './pyodide/bridge.js';
import { formatShort, hh } from './lib/dates.js';
import Sidebar from './components/Sidebar.jsx';
import Login from './components/Login.jsx';
import FloorPlan, { deskColor } from './components/FloorPlan.jsx';
import BookingPanel from './components/BookingPanel.jsx';
import BookingsPage from './components/BookingsPage.jsx';
import UsersPage from './components/UsersPage.jsx';
import AdminPage from './components/AdminPage.jsx';
import { ColleagueSearch, DateControl, LocationBadge, TimeFilter } from './components/TopBar.jsx';

const POLL_MS = 30_000;

export default function App() {
  const [auth, setAuth] = useState(undefined); // undefined: checking, null: signed out

  useEffect(() => {
    setUnauthorizedHandler(() => setAuth(null));
    api
      .session()
      .then((s) => {
        if (!s.user) return setAuth(null);
        setCsrfToken(s.csrfToken);
        setAuth(s);
      })
      .catch(() => setAuth(null));
    loadRules().catch(() => {}); // start fetching Pyodide while the user signs in
  }, []);

  if (auth === undefined) return <div className="flex h-full items-center justify-center text-ink-muted">Loading…</div>;
  if (!auth)
    return (
      <Login
        onSignedIn={(s) => {
          setCsrfToken(s.csrfToken);
          setAuth(s);
        }}
      />
    );
  return (
    <Workspace
      user={auth.user}
      onLogout={async () => {
        await api.logout().catch(() => {});
        setCsrfToken(null);
        setAuth(null);
      }}
    />
  );
}

function defaultHours(config, date) {
  const { openHour, closeHour } = config.settings;
  const min = date === config.today ? Math.max(openHour, config.nowHour) : openHour;
  const start = Math.min(Math.max(9, min), closeHour - 1);
  return [start, Math.min(closeHour, Math.max(start + 1, 17))];
}

function Workspace({ user, onLogout }) {
  const [view, setView] = useState('map');
  const [config, setConfig] = useState(null);
  const [desks, setDesks] = useState([]);
  const [bookings, setBookings] = useState([]);
  const [colleagues, setColleagues] = useState([]);
  const [date, setDate] = useState(null);
  const [hours, setHours] = useState([9, 17]);
  const [selectedDeskId, setSelectedDeskId] = useState(null);
  const [deskStates, setDeskStates] = useState({ states: {}, availableCount: null });
  const [pyStatus, setPyStatus] = useState('loading');
  const [checkErrors, setCheckErrors] = useState([]);
  const [freeSlots, setFreeSlots] = useState([]);
  const [message, setMessage] = useState(null);
  const [submitting, setSubmitting] = useState(false);
  const [highlight, setHighlight] = useState([]);
  const [showSafety, setShowSafety] = useState(true);
  const [loadError, setLoadError] = useState(null);

  /* ---------- data loading ---------- */

  const refreshBookings = useCallback(async (cfg) => {
    const c = cfg || config;
    if (!c) return;
    const { bookings: list } = await api.bookings(c.today, c.lastDate);
    setBookings(list);
  }, [config]);

  const refreshAll = useCallback(async () => {
    try {
      const cfg = await api.config();
      const [{ desks: d }, { colleagues: cl }, { bookings: b }] = await Promise.all([
        api.desks(),
        api.colleagues(),
        api.bookings(cfg.today, cfg.lastDate),
      ]);
      setConfig(cfg);
      setDesks(d);
      setColleagues(cl);
      setBookings(b);
      setLoadError(null);
      setDate((cur) => (!cur || cur < cfg.today || cur > cfg.lastDate ? cfg.today : cur));
    } catch (e) {
      setLoadError(e.message);
    }
  }, []);

  useEffect(() => {
    refreshAll();
    const t = setInterval(refreshAll, POLL_MS); // see colleagues' bookings appear
    const onFocus = () => refreshAll();
    window.addEventListener('focus', onFocus);
    return () => {
      clearInterval(t);
      window.removeEventListener('focus', onFocus);
    };
  }, [refreshAll]);

  useEffect(() => {
    loadRules()
      .then(() => setPyStatus('ready'))
      .catch((e) => {
        console.error(e);
        setPyStatus('error');
      });
  }, []);

  // Reset hours to sensible defaults when the date changes (and on first load).
  const lastDateRef = useRef(null);
  useEffect(() => {
    if (!config || !date || lastDateRef.current === date) return;
    lastDateRef.current = date;
    setHours(defaultHours(config, date));
  }, [config, date]);

  const bookingsOnDate = useMemo(() => bookings.filter((b) => b.date === date), [bookings, date]);
  const [startHour, endHour] = hours;

  /* ---------- Python: desk states, rule checks, free slots ---------- */

  const seq = useRef(0);
  useEffect(() => {
    if (pyStatus !== 'ready' || !config || !date) return;
    const n = ++seq.current;
    rules
      .deskStates({
        desks, bookings: bookingsOnDate, date, startHour, endHour, userId: user.id,
        selectedDeskId, today: config.today, settings: config.settings,
      })
      .then((r) => n === seq.current && setDeskStates(r))
      .catch((e) => console.error(e));
  }, [pyStatus, config, desks, bookingsOnDate, date, startHour, endHour, selectedDeskId, user.id]);

  useEffect(() => {
    if (pyStatus !== 'ready' || !config || !selectedDeskId) {
      setCheckErrors([]);
      setFreeSlots([]);
      return;
    }
    let live = true;
    const request = { deskId: selectedDeskId, date, startHour, endHour };
    Promise.all([
      rules.checkBooking({
        request, settings: config.settings, today: config.today, nowHour: config.nowHour,
        desks, bookings: bookingsOnDate, employeeId: user.id,
      }),
      rules.freeSlots({ deskId: selectedDeskId, date, bookings: bookingsOnDate, settings: config.settings }),
    ])
      .then(([errs, slots]) => {
        if (!live) return;
        setCheckErrors(errs);
        setFreeSlots(
          date === config.today
            ? slots
                .map((s) => ({ ...s, startHour: Math.max(s.startHour, config.nowHour) }))
                .filter((s) => s.startHour < s.endHour)
            : slots,
        );
      })
      .catch((e) => console.error(e));
    return () => {
      live = false;
    };
  }, [pyStatus, config, selectedDeskId, date, startHour, endHour, desks, bookingsOnDate, user.id]);

  /* ---------- actions ---------- */

  const selectedDesk = desks.find((d) => d.id === selectedDeskId);

  const confirmBooking = async () => {
    setSubmitting(true);
    setMessage(null);
    try {
      const { booking } = await api.createBooking({ deskId: selectedDeskId, date, startHour, endHour });
      setMessage({ ok: true, text: `Booked desk ${booking.deskLabel} on ${formatShort(date)}, ${hh(startHour)}–${hh(endHour)}.` });
    } catch (e) {
      setMessage({ ok: false, text: e instanceof ApiError && e.status === 409 ? `${e.message}. Someone else may have just booked it.` : e.message });
    } finally {
      await refreshBookings().catch(() => {});
      setSubmitting(false);
    }
  };

  const cancelBooking = async (b) => {
    if (!window.confirm(`Cancel your booking of ${b.deskLabel} ${hh(b.startHour)}–${hh(b.endHour)}?`)) return;
    setSubmitting(true);
    try {
      await api.cancelBooking(b.id);
      setMessage({ ok: true, text: 'Booking cancelled.' });
    } catch (e) {
      setMessage({ ok: false, text: e.message });
    } finally {
      await refreshBookings().catch(() => {});
      setSubmitting(false);
    }
  };

  const pickColleague = (c) => {
    if (!c) return setHighlight([]);
    const first = c.name.split(' ')[0];
    const hits = bookingsOnDate.filter((b) => b.employeeId === c.id);
    setHighlight(hits.map((b) => ({ deskId: b.deskId, label: first })));
    if (hits[0]) setHours([hits[0].startHour, hits[0].endHour]);
  };

  if (!config || !date) {
    return (
      <div className="flex h-full items-center justify-center text-ink-muted">
        {loadError ? <span className="text-red-700">Cannot load data: {loadError}</span> : 'Loading…'}
      </div>
    );
  }

  const minStart = date === config.today ? config.nowHour : 0;
  const deskBookings = selectedDeskId ? bookingsOnDate.filter((b) => b.deskId === selectedDeskId) : [];

  return (
    <div className="flex h-full">
      <Sidebar view={view} onNavigate={setView} user={user} onLogout={onLogout} />
      <main className="relative flex min-w-0 flex-1">
        {view === 'map' && (
          <>
            <div className="relative min-w-0 flex-1">
              <div className="absolute inset-0 pb-14 pt-20">
                <FloorPlan
                  desks={desks}
                  states={deskStates.states}
                  selectedDeskId={selectedDeskId}
                  highlight={highlight}
                  showSafety={showSafety}
                  extraControls={
                    <button
                      onClick={() => setShowSafety((v) => !v)}
                      className={`rounded-md border border-slate-200 bg-white p-1.5 shadow-card ${showSafety ? '' : 'opacity-50'}`}
                      aria-pressed={showSafety}
                      aria-label="Show first aid, fire equipment and exits"
                      title="Show first aid, fire equipment and exits"
                    >
                      <svg viewBox="0 0 20 20" className="h-5 w-5"><rect width="20" height="20" rx="3" fill="#E5322D" /><path d="M8 4h4v4h4v4h-4v4H8v-4H4V8h4z" fill="#fff" /></svg>
                    </button>
                  }
                  onDeskClick={(id) => {
                    setSelectedDeskId(id);
                    setMessage(null);
                  }}
                />
              </div>

              <header className="absolute left-0 right-0 top-0 z-10 flex items-center gap-4 px-8 py-5">
                <DateControl date={date} today={config.today} lastDate={config.lastDate} onChange={setDate} />
                <TimeFilter
                  startHour={startHour}
                  endHour={endHour}
                  openHour={config.settings.openHour}
                  closeHour={config.settings.closeHour}
                  minStart={minStart}
                  onChange={(s, e) => setHours([s, e])}
                />
                <div className="flex flex-1 justify-center">
                  <LocationBadge office={config.settings.officeName} floor={config.settings.floorName} available={deskStates.availableCount} />
                </div>
                <div className="flex flex-1 justify-end">
                  <ColleagueSearch colleagues={colleagues} bookingsOnDate={bookingsOnDate} onPick={pickColleague} />
                </div>
              </header>

              {pyStatus !== 'ready' && (
                <div className={`absolute left-1/2 top-20 z-10 -translate-x-1/2 rounded-md px-4 py-2 text-sm shadow-card ${pyStatus === 'error' ? 'bg-red-50 text-red-800' : 'bg-white text-ink-muted'}`}>
                  {pyStatus === 'error' ? 'Booking rules engine failed to load. Reload the page; contact IT if it persists.' : 'Loading booking rules…'}
                </div>
              )}

              <div className="absolute bottom-6 left-6 flex flex-wrap gap-3 rounded-md bg-white/90 px-3 py-2 text-xs shadow-card">
                {['available', 'selected', 'booked', 'unavailable'].map((s) => (
                  <span key={s} className="flex items-center gap-1.5 capitalize">
                    <span className="h-3 w-3 rounded-sm" style={{ background: deskColor(s) }} /> {s}
                  </span>
                ))}
                <span className="flex items-center gap-1.5">
                  <span className="rounded-full bg-brand px-1.5 text-[10px] font-bold text-white">YOU</span> your desk
                </span>
              </div>

            </div>

            {selectedDesk && (
              <BookingPanel
                desk={selectedDesk}
                deskState={deskStates.states[selectedDesk.id]}
                date={date}
                startHour={startHour}
                endHour={endHour}
                deskBookings={deskBookings}
                freeSlots={freeSlots}
                errors={pyStatus === 'ready' ? checkErrors : []}
                submitting={submitting}
                message={message}
                onConfirm={confirmBooking}
                onCancelBooking={cancelBooking}
                onPickSlot={(s) => setHours([s.startHour, s.endHour])}
                onClose={() => {
                  setSelectedDeskId(null);
                  setMessage(null);
                }}
              />
            )}
          </>
        )}

        {view === 'bookings' && (
          <div className="flex-1">
            <BookingsPage
              config={config}
              user={user}
              desks={desks}
              onChanged={() => refreshBookings().catch(() => {})}
              onShowOnMap={(b) => {
                lastDateRef.current = b.date;
                setDate(b.date);
                setHours([b.startHour, b.endHour]);
                setSelectedDeskId(b.deskId);
                setView('map');
              }}
            />
          </div>
        )}

        {view === 'users' && (
          <div className="flex-1">
            <UsersPage
              colleagues={colleagues}
              bookingsOnDate={bookingsOnDate}
              date={date}
              onShowOnMap={(c) => {
                pickColleague(c);
                setView('map');
              }}
            />
          </div>
        )}

        {view === 'admin' && user.role === 'admin' && (
          <div className="min-w-0 flex-1">
            <AdminPage
              me={user}
              desks={desks}
              settings={config.settings}
              onDesksChanged={refreshAll}
              onSettingsChanged={refreshAll}
            />
          </div>
        )}
      </main>
    </div>
  );
}
