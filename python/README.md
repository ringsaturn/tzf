# TZF's Python Binding

## Install

```bash
pip install tzfpy
```

## Usage

```py
>>> from tzfpy import get_tz
>>> print(get_tz(121.4737, 31.2305))
Asia/Shanghai
```

## For dev

```bash
# local install
pip install -e . 

# lcoal wheel
python -m build --wheel .
```
