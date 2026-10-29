"""Availability calculations for the floor plan."""
from __future__ import annotations

from datetime import date

from .intervals import Interval, free_slots
from .policy import DEFAULT_SETTINGS, conflicts, is_past, window_dates
from .intervals import parse_date


def desk_states(*, desks: list[dict], bookings: list[dict], day: str, start: int, end: int,
                user_id: str | None, selected_desk_id: str | None = None,
                today: date | None = None, settings: dict | None = None) -> dict:
    """Map desk id -> {state, bookedBy, mine}.

    state is one of: available, selected, booked, unavailable.
    """
    s = {**DEFAULT_SETTINGS, **(settings or {})}
    out_of_window = False
    if today is not None:
        first, last = window_dates(today, s["windowDays"])
        out_of_window = not first <= parse_date(day) <= last
    result = {}
    for desk in desks:
        hits = conflicts(desk["id"], day, start, end, bookings)
        mine = any(b.get("employeeId") == user_id for b in hits)
        if not desk.get("active", True) or out_of_window:
            state = "unavailable"
        elif hits:
            state = "booked"
        elif desk["id"] == selected_desk_id:
            state = "selected"
        else:
            state = "available"
        result[desk["id"]] = {
            "state": state,
            "mine": mine,
            "bookedBy": [b.get("employeeName", b.get("employeeId")) for b in hits],
        }
    return result


def available_count(states: dict) -> int:
    return sum(1 for v in states.values() if v["state"] in ("available", "selected"))


def desk_free_slots(desk_id: str, day: str, bookings: list[dict], settings: dict | None = None) -> list[dict]:
    s = {**DEFAULT_SETTINGS, **(settings or {})}
    busy = [Interval(b["startHour"], b["endHour"]) for b in
            conflicts(desk_id, day, s["openHour"], s["closeHour"], bookings)]
    return [{"startHour": i.start, "endHour": i.end} for i in free_slots(s["openHour"], s["closeHour"], busy)]


def split_my_bookings(bookings: list[dict], user_id: str, today: date, now_hour: int) -> dict:
    mine = [b for b in bookings if b.get("employeeId") == user_id]
    key = lambda b: (b["date"], b["startHour"])
    upcoming = sorted((b for b in mine if b.get("status", "active") == "active" and not is_past(b, today, now_hour)), key=key)
    past = sorted((b for b in mine if b not in upcoming), key=key, reverse=True)
    return {"upcoming": upcoming, "past": past}
