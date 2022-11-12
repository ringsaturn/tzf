import os
from ctypes import (
    CDLL,
    POINTER,
    c_char_p,
    c_float,
    c_int,
    c_long,
    c_void_p,
    string_at,
)
from typing import List

c_lib = CDLL(os.path.join(os.path.dirname(__file__), "tzf.so"))

_get = c_lib.GetTZ
_get.argtypes = [POINTER(c_float), POINTER(c_float)]
_get.restype = c_void_p

_names_counts = c_lib.CountTimezoneNames
_names_counts.restype = c_long

_names = c_lib.TimezoneNames
_names.restype = POINTER(c_char_p)

# For free mem
_free = c_lib.FreeChar
_free.argtypes = [c_void_p]
_free.restype = c_int

_count = _names_counts()


def _setup_timezone_names() -> List[str]:
    res = _names()
    names = []
    for i in range(_count):
        names.append(res[i].decode())
    return names


names = _setup_timezone_names()


def get_tz(lng: float, lat: float) -> str:
    raw = _get(c_float(lng), c_float(lat))
    ret = string_at(raw).decode()
    _free(raw)
    return ret


def timezone_names() -> List[str]:
    return names


def timezone_names_counts() -> int:
    return _count


if __name__ == "__main__":
    print(get_tz(11, 11))
    print(get_tz(116, 39))
    print(timezone_names())
