"""Input validation for booking requests coming from the UI.

Returns a list of ``{"field", "message"}`` errors instead of raising, so the
UI can show all problems at once. The Go server re-validates independently.
"""
from __future__ import annotations

import re

from .intervals import parse_date

_ID_RE = re.compile(r"^[A-Za-z0-9_-]{1,64}$")


def _is_int(v) -> bool:
    return isinstance(v, int) and not isinstance(v, bool)


def validate_booking_request(req: dict) -> list[dict]:
    errors: list[dict] = []
    if not isinstance(req, dict):
        return [{"field": "_", "message": "request must be an object"}]

    desk_id = req.get("deskId")
    if not isinstance(desk_id, str) or not _ID_RE.match(desk_id):
        errors.append({"field": "deskId", "message": "invalid desk id"})

    try:
        parse_date(req.get("date"))
    except (TypeError, ValueError):
        errors.append({"field": "date", "message": "date must be YYYY-MM-DD"})

    start, end = req.get("startHour"), req.get("endHour")
    hours_ok = True
    for name, v in (("startHour", start), ("endHour", end)):
        if not _is_int(v) or not 0 <= v <= 24:
            errors.append({"field": name, "message": "hour must be a whole number 0-24"})
            hours_ok = False
    if hours_ok and end <= start:
        errors.append({"field": "endHour", "message": "end time must be later than start time"})
    return errors
