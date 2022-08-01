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


------------------------------------------------ benchmark: 1 tests ------------------------------------------------
Name (time in us)         Min      Max     Mean  StdDev   Median     IQR  Outliers  OPS (Kops/s)  Rounds  Iterations
--------------------------------------------------------------------------------------------------------------------
test_tzfpy_random     17.3780  92.8220  30.5366  7.3196  30.2675  3.7870     59;57       32.7476     308           1
--------------------------------------------------------------------------------------------------------------------

Legend:
  Outliers: 1 Standard Deviation from Mean; 1.5 IQR (InterQuartile Range) from 1st Quartile and 3rd Quartile.
  OPS: Operations Per Second, computed as 1 / Mean
```
