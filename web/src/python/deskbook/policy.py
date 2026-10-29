"""Booking policy rules.

All dates/hours are office-local. ``today`` and ``now_hour`` are always passed
in (the browser takes them from the server's /api/config so every client uses
the office timezone, never the PC clock's zone). That keeps these functions
pure and testable.

Rolling window: a booking date must satisfy ``today <= date < today + window``
(window defaults to 60, so today plus the next 59 days).
"""
from __future__ import annotations

from datetime import date, timedelta

from .intervals import overlaps, parse_date
from .validation import validate_booking_request

DEFAULT_SETTINGS = {"windowDays": 60, "openHour": 7, "closeHour": 19}


def window_dates(today: date, window_days: int) -> tuple[date, date]:
    """First and last bookable date (inclusive)."""
    return today, today + timedelta(days=window_days - 1)


def check_booking(req: dict, *, settings: dict, today: date, now_hour: int,
                  desks: list[dict], bookings: list[dict],
                  ignore_booking_id: str | None = None,
                  employee_id: str | None = None) -> list[dict]:
    """Return policy errors for a new/changed booking (empty list = OK).

    If ``employee_id`` is given, the employee may not hold two desks at once.
    """
    errors = validate_booking_request(req)
    if errors:
        return errors
    s = {**DEFAULT_SETTINGS, **(settings or {})}
    d = parse_date(req["date"])
    first, last = window_dates(today, s["windowDays"])
    if d < first:
        errors.append({"field": "date", "message": "date is in the past"})
    elif d > last:
        errors.append({"field": "date", "message": f"date is beyond the {s['windowDays']}-day booking window"})

    start, end = req["startHour"], req["endHour"]
    if start < s["openHour"] or end > s["closeHour"]:
        errors.append({"field": "startHour",
                       "message": f"bookings must be within {s['openHour']:02d}:00-{s['closeHour']:02d}:00"})
    if d == today and start < now_hour:
        errors.append({"field": "startHour", "message": "start time has already passed"})

    desk = next((x for x in desks if x.get("id") == req["deskId"]), None)
    if desk is None:
        errors.append({"field": "deskId", "message": "desk does not exist"})
    elif not desk.get("active", True):
        errors.append({"field": "deskId", "message": "desk is unavailable"})

    for b in conflicts(req["deskId"], req["date"], start, end, bookings, ignore_booking_id):
        errors.append({"field": "deskId",
                       "message": f"desk already booked {b['startHour']:02d}:00-{b['endHour']:02d}:00"})
    if employee_id is not None:
        for b in bookings:
            if (b.get("employeeId") == employee_id and b.get("date") == req["date"]
                    and b.get("status", "active") == "active" and b.get("id") != ignore_booking_id
                    and overlaps(start, end, b["startHour"], b["endHour"])):
                errors.append({"field": "startHour", "message": "you already have a booking at that time"})
                break
    return errors


def conflicts(desk_id: str, day: str, start: int, end: int, bookings: list[dict],
              ignore_booking_id: str | None = None) -> list[dict]:
    return [b for b in bookings
            if b.get("deskId") == desk_id and b.get("date") == day
            and b.get("status", "active") == "active"
            and b.get("id") != ignore_booking_id
            and overlaps(start, end, b["startHour"], b["endHour"])]


def can_modify(booking: dict, user: dict) -> bool:
    """Owners may cancel their own bookings; admins may cancel any."""
    return user.get("role") == "admin" or booking.get("employeeId") == user.get("id")


def is_past(booking: dict, today: date, now_hour: int) -> bool:
    d = parse_date(booking["date"])
    return d < today or (d == today and booking["endHour"] <= now_hour)
