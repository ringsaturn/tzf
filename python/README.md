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

## Performance

tzfpy is fast and stable, test under *2.3 GHz 8-Core Intel Core i9*:


```
tzfpy/tests.py ..                                                                                                                                                      [100%]


------------------------------------------------- benchmark: 1 tests -------------------------------------------------
Name (time in us)         Min       Max     Mean   StdDev   Median     IQR  Outliers  OPS (Kops/s)  Rounds  Iterations
----------------------------------------------------------------------------------------------------------------------
test_tzfpy_random     31.3690  220.0310  45.5814  11.6420  44.7730  3.7180   366;788       21.9388    5322           1
----------------------------------------------------------------------------------------------------------------------

Legend:
  Outliers: 1 Standard Deviation from Mean; 1.5 IQR (InterQuartile Range) from 1st Quartile and 3rd Quartile.
  OPS: Operations Per Second, computed as 1 / Mean
```
