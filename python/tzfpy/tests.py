from unittest import TestCase, main

from tzfpy import get_tz


class TestTZF(TestCase):
    def test_shanghai(self):
        self.assertEqual(get_tz(121.4737, 31.2305), "Asia/Shanghai")


def _test_get_tz():
    _ = get_tz(lng=13.358, lat=52.5061)
    _ = get_tz(lng=116, lat=39)
    _ = get_tz(lng=0.1276, lat=51.5073)


def test_tz(benchmark):
    benchmark(_test_get_tz)


if __name__ == "__main__":
    main()
