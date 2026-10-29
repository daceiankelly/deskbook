"""JSON-in / JSON-out entry points called from JavaScript through Pyodide.

Every function takes one JSON string and returns one JSON string, so the JS
side never handles Python proxies. See web/src/pyodide/README.md.
"""
from __future__ import annotations

import json
from datetime import date

from . import availability, policy, validation


def _call(fn, payload: str) -> str:
    try:
        return json.dumps({"ok": True, "result": fn(json.loads(payload))})
    except Exception as exc:  # surfaced to the UI, never crashes the runtime
        return json.dumps({"ok": False, "error": str(exc)})


def validate_booking(payload: str) -> str:
    return _call(lambda p: validation.validate_booking_request(p["request"]), payload)


def check_booking(payload: str) -> str:
    return _call(lambda p: policy.check_booking(
        p["request"], settings=p["settings"], today=date.fromisoformat(p["today"]),
        now_hour=p["nowHour"], desks=p["desks"], bookings=p["bookings"],
        ignore_booking_id=p.get("ignoreBookingId"), employee_id=p.get("employeeId")), payload)


def desk_states(payload: str) -> str:
    def run(p):
        states = availability.desk_states(
            desks=p["desks"], bookings=p["bookings"], day=p["date"], start=p["startHour"],
            end=p["endHour"], user_id=p.get("userId"), selected_desk_id=p.get("selectedDeskId"),
            today=date.fromisoformat(p["today"]) if p.get("today") else None, settings=p.get("settings"))
        return {"states": states, "availableCount": availability.available_count(states)}
    return _call(run, payload)


def free_slots(payload: str) -> str:
    return _call(lambda p: availability.desk_free_slots(p["deskId"], p["date"], p["bookings"], p.get("settings")), payload)


def my_bookings(payload: str) -> str:
    return _call(lambda p: availability.split_my_bookings(
        p["bookings"], p["userId"], date.fromisoformat(p["today"]), p["nowHour"]), payload)


def window_dates(payload: str) -> str:
    def run(p):
        first, last = policy.window_dates(date.fromisoformat(p["today"]), p["windowDays"])
        return {"first": first.isoformat(), "last": last.isoformat()}
    return _call(run, payload)
