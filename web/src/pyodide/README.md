# JavaScript ⇄ Pyodide interface

`bridge.js` loads a **locally hosted** Pyodide (`/pyodide/`, copied from
`node_modules/pyodide` by `scripts/copy-pyodide.mjs`) and imports the
`deskbook` Python package from `src/python/deskbook`.

```js
import { loadRules, rules } from './pyodide/bridge';

await loadRules();                       // optional warm-up; first call does it too
const errors = await rules.checkBooking({ request, settings, today, nowHour, desks, bookings });
```

## Contract

* Each Python entry point in `deskbook/bridge.py` takes **one JSON string** and
  returns **one JSON string** `{"ok": true, "result": …}` or
  `{"ok": false, "error": "…"}`. No Python proxy objects cross into React.
* `rules.*` wrappers parse the result and throw `Error` when `ok` is false.
* Dates are office-local `YYYY-MM-DD` strings; hours are integers 0–24.
  Intervals are half-open `[startHour, endHour)`, so back-to-back bookings
  never overlap.
* `today` and `nowHour` must come from the server (`GET /api/config`), never
  from the browser clock, so all PCs use the configured office timezone.

| JS function | Python | Payload | Result |
|---|---|---|---|
| `validateBooking` | `validate_booking` | `{request}` | `[{field, message}]` |
| `checkBooking` | `check_booking` | `{request, settings, today, nowHour, desks, bookings, ignoreBookingId?, employeeId?}` | `[{field, message}]` (empty = OK) |
| `deskStates` | `desk_states` | `{desks, bookings, date, startHour, endHour, userId, selectedDeskId?, today?, settings?}` | `{states: {deskId: {state, mine, bookedBy}}, availableCount}` |
| `freeSlots` | `free_slots` | `{deskId, date, bookings, settings?}` | `[{startHour, endHour}]` |
| `myBookings` | `my_bookings` | `{bookings, userId, today, nowHour}` | `{upcoming, past}` |
| `windowDates` | `window_dates` | `{today, windowDays}` | `{first, last}` |

`request` = `{deskId, date, startHour, endHour}`.

## Trust boundary

These checks give instant feedback only. The Go API re-validates every
create/change/cancel against the latest stored data and is the sole authority.

## Testing without a browser

The package is plain Python 3.10+ with no dependencies:

```bash
cd web/src/python
python3 -m unittest discover -s tests -v
```
