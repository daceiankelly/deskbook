# Testing

```bash
cd web/src/python && python3 -m unittest discover -s tests -v   # Python rules
cd server && go vet ./... && go test -race -count=1 ./...        # Go store, rules, API
cd web && npm run build                                          # build + static offline check
```

## Requirement → test map

| Requirement | Automated test |
|---|---|
| Concurrent booking requests | `api.TestConcurrentBookingsOnlyOneWins` (50 simultaneous requests, 10 users, same desk/time → exactly one 201, 49× 409, one record on disk); `TestConcurrentNonOverlappingAllSucceed` |
| Overlapping & back-to-back | `api.TestOverlapBackToBackAndRanges`, `booking.TestValidate`, `store.TestRejectsOverlappingBookingsInFile`, Python `test_overlap_rejected_back_to_back_allowed` |
| Invalid date ranges | `booking.TestValidate` (end ≤ start, bad dates, out-of-range hours), Python `ValidationTests`, API steps 6–7 |
| 60-day boundaries | `booking.TestValidate` (day 60 OK, day 61 rejected, yesterday rejected), `TestOfficeTimezoneDecidesToday`, `TestWindowAcrossDST`, Python `test_window_boundaries` |
| Authentication | `api.TestAuthenticationRequired`, `TestLoginFailuresAndRateLimit`, `TestCSRFAndOriginEnforced` |
| Authorization | `api.TestClientCannotChooseEmployee`, `TestCancellationAndOwnership`, `TestAdminAuthorization` |
| Cancellation | `api.TestCancellationAndOwnership`, `TestCannotCancelEndedBooking` |
| Corrupt / missing JSON | `store.TestRejectsMalformedAndWrongSchema` (garbage, truncated, empty, no/future version, unknown field, trailing data, bad timezone — file left untouched), `TestMissingFileWithBackupsRefusesToReseed`, `TestRefusesWriteWhenDiskCorruptedWhileRunning` |
| Interrupted writes & recovery | `store.TestInterruptedWriteKeepsOriginal` (failure injected between fsync and rename), `TestCrashLeftoverTempIgnored`, `TestBackupsRotateAndRecover`, `TestRestoreRejectsInvalidBackupAndRunningServer` |
| Storage failure never reported as success | `api.TestStorageFailureIsNotReportedAsSuccess` |
| Startup removal of previous days' bookings | `booking.TestPurgeBeforeToday` (office-timezone midnight), `main.TestPurgePastBookingsOnStart` (keeps today incl. ended + future, persisted, no rewrite when nothing to remove) |
| Persistence after restart | `store.TestPersistenceAcrossRestart`, `api.TestPersistenceAfterServerRestart` |
| Single instance | `store.TestSecondInstanceIsLocked` |
| Data file not exposed | `main.TestStaticHandlerNeverExposesData` |
| Operation without internet | `npm run check-offline` (static) + CSP `connect-src 'self'` + manual procedure below |

## Manual acceptance checklist

Run on the real office network after deployment.

1. **Offline:** disconnect the server's and a client's internet uplink (or block
   it at the router), keep the LAN up. Clear the browser cache, open the app,
   sign in. DevTools → Network: every request goes to the server's LAN address;
   Console shows no CSP violations; the "Loading booking rules…" banner disappears
   (Pyodide loaded from `/pyodide/`).
2. **Two PCs, same desk:** two employees select the same desk/time and press
   Confirm together. One sees success, the other "desk already booked…".
3. **Back-to-back:** book 09–12, then another employee books 12–14 on the same desk.
4. **Window:** the date picker stops at day 60; a crafted request for day 61
   (`curl` with a valid session) returns 422.
5. **Restart:** `systemctl restart deskbook`; after signing in again all bookings are present.
6. **Power cut simulation:** `kill -9` the server during a busy period; it restarts
   and `deskbook verify` passes on the data file.
7. **Recovery drill:** stop the server, corrupt `deskbook.json`
   (`echo x > deskbook.json`), start (it refuses), run `deskbook recover`, start again.
8. **CSP:** the default policy has no `'unsafe-eval'`. Open the app in every
   browser the office uses (Chromium is verified; check Firefox and Edge) and
   confirm the "Loading booking rules…" banner disappears with no CSP errors in
   the console. Only if one fails, start the server with `-allow-unsafe-eval`.
