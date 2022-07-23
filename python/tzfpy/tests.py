from unittest import TestCase, main

from tzfpy import get_tz


class TestTZF(TestCase):
    def test_shanghai(self):
        self.assertEqual(get_tz(121.4737, 31.2305), "Asia/Shanghai")


if __name__ == "__main__":
    main()
