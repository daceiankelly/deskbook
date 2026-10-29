// JavaScript -> Pyodide interface. See ./README.md for the contract.
//
// The Python package in src/python/deskbook is bundled as raw text by Vite,
// written into Pyodide's in-memory filesystem, and imported once. Every call
// sends one JSON string and receives one JSON string {ok, result | error}.

const pySources = import.meta.glob('../python/deskbook/*.py', {
  query: '?raw',
  import: 'default',
  eager: true,
});

const PYODIDE_BASE = `${import.meta.env.BASE_URL}pyodide/`;

let loading = null;

export function loadRules() {
  if (!loading) {
    loading = (async () => {
      // Served by our own Go server from dist/pyodide (copied from node_modules).
      const { loadPyodide } = await import(/* @vite-ignore */ `${PYODIDE_BASE}pyodide.mjs`);
      const pyodide = await loadPyodide({ indexURL: PYODIDE_BASE });
      pyodide.FS.mkdirTree('/home/pyodide/deskbook');
      for (const [path, source] of Object.entries(pySources)) {
        const name = path.split('/').pop();
        pyodide.FS.writeFile(`/home/pyodide/deskbook/${name}`, source);
      }
      pyodide.runPython('import sys\nif "/home/pyodide" not in sys.path: sys.path.insert(0, "/home/pyodide")');
      return pyodide.pyimport('deskbook.bridge');
    })();
    loading.catch(() => {
      loading = null; // allow a retry after a failed load
    });
  }
  return loading;
}

async function call(fn, payload) {
  const bridge = await loadRules();
  const out = JSON.parse(bridge[fn](JSON.stringify(payload)));
  if (!out.ok) throw new Error(`rules.${fn}: ${out.error}`);
  return out.result;
}

/** Business rules implemented in Python. All functions are async. */
export const rules = {
  /** {request} -> [{field, message}] */
  validateBooking: (p) => call('validate_booking', p),
  /** {request, settings, today, nowHour, desks, bookings, ignoreBookingId?, employeeId?} -> [{field, message}] */
  checkBooking: (p) => call('check_booking', p),
  /** {desks, bookings, date, startHour, endHour, userId, selectedDeskId?, today?, settings?} -> {states, availableCount} */
  deskStates: (p) => call('desk_states', p),
  /** {deskId, date, bookings, settings?} -> [{startHour, endHour}] */
  freeSlots: (p) => call('free_slots', p),
  /** {bookings, userId, today, nowHour} -> {upcoming, past} */
  myBookings: (p) => call('my_bookings', p),
  /** {today, windowDays} -> {first, last} */
  windowDates: (p) => call('window_dates', p),
};
