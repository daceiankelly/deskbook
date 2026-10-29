"""Time interval calculations.

Bookings are whole-hour, half-open intervals ``[start, end)`` on a single
office-local calendar date. Half-open means 09-10 and 10-11 do not overlap,
which is what permits back-to-back bookings.
"""
from __future__ import annotations

from dataclasses import dataclass
from datetime import date, timedelta


@dataclass(frozen=True, order=True)
class Interval:
    start: int  # hour of day, inclusive
    end: int    # hour of day, exclusive

    def __post_init__(self):
        if self.end <= self.start:
            raise ValueError("end must be later than start")

    @property
    def hours(self) -> int:
        return self.end - self.start

    def overlaps(self, other: "Interval") -> bool:
        return self.start < other.end and other.start < self.end

    def contains(self, other: "Interval") -> bool:
        return self.start <= other.start and other.end <= self.end


def overlaps(a_start: int, a_end: int, b_start: int, b_end: int) -> bool:
    """Half-open overlap test; identical rule to the Go server."""
    return a_start < b_end and b_start < a_end


def merge(intervals: list[Interval]) -> list[Interval]:
    """Merge overlapping or touching intervals into a sorted minimal list."""
    out: list[Interval] = []
    for iv in sorted(intervals):
        if out and iv.start <= out[-1].end:
            last = out[-1]
            out[-1] = Interval(last.start, max(last.end, iv.end))
        else:
            out.append(iv)
    return out


def free_slots(open_hour: int, close_hour: int, busy: list[Interval]) -> list[Interval]:
    """Gaps inside opening hours not covered by ``busy``."""
    free: list[Interval] = []
    cursor = open_hour
    for iv in merge(busy):
        if iv.end <= open_hour or iv.start >= close_hour:
            continue
        if iv.start > cursor:
            free.append(Interval(cursor, min(iv.start, close_hour)))
        cursor = max(cursor, iv.end)
    if cursor < close_hour:
        free.append(Interval(cursor, close_hour))
    return free


def parse_date(value: str) -> date:
    """Parse strict ``YYYY-MM-DD``."""
    if not isinstance(value, str) or len(value) != 10:
        raise ValueError("date must be YYYY-MM-DD")
    return date.fromisoformat(value)


def date_range(start: date, days: int) -> list[date]:
    return [start + timedelta(days=i) for i in range(days)]
