"""Run from web/src/python:  python -m unittest discover -s tests -v"""
import json
import os
import sys
import unittest
from datetime import date

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from deskbook import availability, bridge, intervals, policy, validation  # noqa: E402

TODAY = date(2026, 10, 28)
SETTINGS = {"windowDays": 60, "openHour": 7, "closeHour": 19}
DESKS = [{"id": "D-01", "active": True}, {"id": "D-02", "active": True}, {"id": "D-03", "active": False}]


def req(**kw):
    base = {"deskId": "D-01", "date": "2026-10-29", "startHour": 9, "endHour": 12}
    base.update(kw)
    return base


def booking(id_, desk="D-01", day="2026-10-29", s=9, e=12, emp="u1", status="active"):
    return {"id": id_, "deskId": desk, "date": day, "startHour": s, "endHour": e,
            "employeeId": emp, "status": status}


def check(r, bookings=(), now_hour=8, **kw):
    return policy.check_booking(r, settings=SETTINGS, today=TODAY, now_hour=now_hour,
                                desks=DESKS, bookings=list(bookings), **kw)


class IntervalTests(unittest.TestCase):
    def test_overlap_and_back_to_back(self):
        self.assertTrue(intervals.overlaps(9, 12, 11, 13))
        self.assertTrue(intervals.overlaps(9, 12, 10, 11))
        self.assertFalse(intervals.overlaps(9, 12, 12, 13))
        self.assertFalse(intervals.overlaps(12, 13, 9, 12))

    def test_interval_rejects_empty_or_inverted(self):
        with self.assertRaises(ValueError):
            intervals.Interval(10, 10)
        with self.assertRaises(ValueError):
            intervals.Interval(11, 10)

    def test_merge_and_free_slots(self):
        busy = [intervals.Interval(9, 10), intervals.Interval(10, 12), intervals.Interval(14, 15)]
        self.assertEqual(intervals.merge(busy), [intervals.Interval(9, 12), intervals.Interval(14, 15)])
        free = intervals.free_slots(7, 19, busy)
        self.assertEqual([(i.start, i.end) for i in free], [(7, 9), (12, 14), (15, 19)])

    def test_parse_date_strict(self):
        for bad in ["2026-1-01", "2026/10/29", "", None, "2026-02-30"]:
            with self.assertRaises((ValueError, TypeError)):
                intervals.parse_date(bad)


class ValidationTests(unittest.TestCase):
    def test_valid(self):
        self.assertEqual(validation.validate_booking_request(req()), [])

    def test_invalid_range(self):
        errs = validation.validate_booking_request(req(startHour=12, endHour=12))
        self.assertEqual(errs[0]["field"], "endHour")
        errs = validation.validate_booking_request(req(startHour=13, endHour=9))
        self.assertEqual(errs[0]["field"], "endHour")

    def test_types(self):
        self.assertTrue(validation.validate_booking_request(req(startHour="9")))
        self.assertTrue(validation.validate_booking_request(req(startHour=True)))
        self.assertTrue(validation.validate_booking_request(req(startHour=25)))
        self.assertTrue(validation.validate_booking_request(req(deskId="../etc")))
        self.assertTrue(validation.validate_booking_request("nope"))


class PolicyTests(unittest.TestCase):
    def test_window_boundaries(self):
        first, last = policy.window_dates(TODAY, 60)
        self.assertEqual(first, date(2026, 10, 28))
        self.assertEqual(last, date(2026, 12, 26))
        self.assertEqual(check(req(date="2026-12-26")), [])
        self.assertEqual(check(req(date="2026-12-27"))[0]["field"], "date")
        self.assertEqual(check(req(date="2026-10-27"))[0]["message"], "date is in the past")

    def test_same_day_past_hour(self):
        self.assertTrue(check(req(date="2026-10-28", startHour=9, endHour=10), now_hour=10))
        self.assertEqual(check(req(date="2026-10-28", startHour=10, endHour=11), now_hour=10), [])

    def test_opening_hours(self):
        self.assertTrue(check(req(startHour=6, endHour=8)))
        self.assertTrue(check(req(startHour=18, endHour=20)))

    def test_overlap_rejected_back_to_back_allowed(self):
        existing = [booking("b1", s=9, e=12, emp="u2")]
        self.assertTrue(check(req(startHour=11, endHour=13), existing))
        self.assertTrue(check(req(startHour=8, endHour=10), existing))
        self.assertEqual(check(req(startHour=12, endHour=14), existing), [])
        self.assertEqual(check(req(startHour=7, endHour=9), existing), [])

    def test_cancelled_and_other_desk_ignored(self):
        existing = [booking("b1", status="cancelled"), booking("b2", desk="D-02")]
        self.assertEqual(check(req(), existing), [])

    def test_change_ignores_itself(self):
        existing = [booking("b1", s=9, e=12)]
        self.assertEqual(check(req(startHour=10, endHour=13), existing, ignore_booking_id="b1"), [])

    def test_inactive_and_unknown_desk(self):
        self.assertTrue(check(req(deskId="D-03")))
        self.assertTrue(check(req(deskId="D-99")))

    def test_employee_double_booking(self):
        existing = [booking("b1", desk="D-02", emp="u1")]
        self.assertTrue(check(req(), existing, employee_id="u1"))
        self.assertEqual(check(req(), existing, employee_id="u2"), [])

    def test_can_modify(self):
        b = booking("b1", emp="u1")
        self.assertTrue(policy.can_modify(b, {"id": "u1", "role": "employee"}))
        self.assertFalse(policy.can_modify(b, {"id": "u2", "role": "employee"}))
        self.assertTrue(policy.can_modify(b, {"id": "u2", "role": "admin"}))


class AvailabilityTests(unittest.TestCase):
    def test_states(self):
        bookings = [booking("b1", desk="D-02", emp="me")]
        st = availability.desk_states(desks=DESKS, bookings=bookings, day="2026-10-29", start=10, end=11,
                                      user_id="me", selected_desk_id="D-01", today=TODAY)
        self.assertEqual(st["D-01"]["state"], "selected")
        self.assertEqual(st["D-02"]["state"], "booked")
        self.assertTrue(st["D-02"]["mine"])
        self.assertEqual(st["D-03"]["state"], "unavailable")
        self.assertEqual(availability.available_count(st), 1)

    def test_out_of_window_unavailable(self):
        st = availability.desk_states(desks=DESKS, bookings=[], day="2027-01-30", start=9, end=10,
                                      user_id="me", today=TODAY)
        self.assertEqual(st["D-01"]["state"], "unavailable")

    def test_free_slots(self):
        slots = availability.desk_free_slots("D-01", "2026-10-29", [booking("b1", s=9, e=12)], SETTINGS)
        self.assertEqual(slots, [{"startHour": 7, "endHour": 9}, {"startHour": 12, "endHour": 19}])

    def test_split_my_bookings(self):
        bs = [booking("past", day="2026-10-27", emp="me"), booking("today-done", day="2026-10-28", s=7, e=9, emp="me"),
              booking("future", emp="me"), booking("cx", emp="me", status="cancelled"), booking("other", emp="x")]
        out = availability.split_my_bookings(bs, "me", TODAY, 10)
        self.assertEqual([b["id"] for b in out["upcoming"]], ["future"])
        self.assertEqual({b["id"] for b in out["past"]}, {"past", "today-done", "cx"})


class BridgeTests(unittest.TestCase):
    def test_json_roundtrip(self):
        out = json.loads(bridge.check_booking(json.dumps({
            "request": req(startHour=13, endHour=12), "settings": SETTINGS, "today": "2026-10-28",
            "nowHour": 8, "desks": DESKS, "bookings": []})))
        self.assertTrue(out["ok"])
        self.assertEqual(out["result"][0]["field"], "endHour")

    def test_errors_are_reported_not_raised(self):
        out = json.loads(bridge.window_dates("{bad json"))
        self.assertFalse(out["ok"])


if __name__ == "__main__":
    unittest.main()
