from unittest import TestCase, main

import numpy as np
from pytz import timezone
from tzfpy import get_tz, timezone_names


class TestTZF(TestCase):
    def test_shanghai(self):
        self.assertEqual(get_tz(121.4737, 31.2305), "Asia/Shanghai")

    def test_names_in_pytz(self):
        list(map(timezone, timezone_names()))


lng_ranges = np.arange(-180, 180, 0.5)
lat_ranges = np.arange(-60, 60, 0.5)


def random_point():
    return np.random.choice(lng_ranges), np.random.choice(lat_ranges)


def _test_tzfpy_random():
    lng, lat = random_point()
    _ = get_tz(lng, lat)


def test_tzfpy_random(benchmark):
    benchmark(_test_tzfpy_random)


def _test_iter_global():
    for lng in lng_ranges:
        for lat in lat_ranges:
            _ = get_tz(lng, lat)


def test_iter_global(benchmark):
    benchmark(_test_iter_global)


if __name__ == "__main__":
    main()
