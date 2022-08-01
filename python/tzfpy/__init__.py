import os
from ctypes import CDLL, POINTER, c_char_p, c_float
from typing import List

c_lib = CDLL(os.path.join(os.path.dirname(__file__), "tzf.so"))

_get = c_lib.GetTZ
_get.argtypes = [POINTER(c_float), POINTER(c_float)]
_get.restype = c_char_p


_names = c_lib.TimezoneNames
_names.restype = POINTER(c_char_p)


def get_tz(lng: float, lat: float) -> str:
    return _get(c_float(lng), c_float(lat)).decode()


def timezone_names() -> List[str]:
    res = _names()
    names = []
    for i in range(450):
        names.append(res[i].decode())
    return names


if __name__ == "__main__":
    print(get_tz(11, 11))
    print(get_tz(116, 39))
    print(timezone_names())
