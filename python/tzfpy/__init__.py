from ctypes import CDLL, POINTER, c_char_p, c_float

c_lib = CDLL("./tzf.so")

_get = c_lib.GetTZ
_get.argtypes = [POINTER(c_float), POINTER(c_float)]
_get.restype = c_char_p


def get_tz(lng: float, lat: float) -> str:
    return _get(c_float(lng), c_float(lat)).decode()


if __name__ == "__main__":
    print(get_tz(11, 11))
    print(get_tz(116, 39))
