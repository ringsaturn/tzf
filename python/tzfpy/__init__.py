import os
from ctypes import CDLL, POINTER, c_char_p, c_float, c_long
from typing import List

c_lib = CDLL(os.path.join(os.path.dirname(__file__), "tzf.so"))

_get = c_lib.GetTZ
_get.argtypes = [POINTER(c_float), POINTER(c_float)]
_get.restype = c_char_p


_names_counts = c_lib.CountTimezoneNames
_names_counts.restype = c_long

_names = c_lib.TimezoneNames
_names.restype = POINTER(c_char_p)


_count = _names_counts()


def _setup_timezone_names() -> List[str]:
    res = _names()
    names = []
    for i in range(_count):
        names.append(res[i].decode())
    return names


names = _setup_timezone_names()


def get_tz(lng: float, lat: float) -> str:
    return _get(c_float(lng), c_float(lat)).decode()


def timezone_names() -> List[str]:
    return names


def timezone_names_counts() -> int:
    return _count


if __name__ == "__main__":
    import time
    while True:
        for i in range(1000):
            get_tz(116, 39)
            timezone_names()
            timezone_names_counts()
        time.sleep(0.01)
    # print(timezone_names())
