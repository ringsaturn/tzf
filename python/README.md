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

`tzfpy` is fast and stable but requires more memory compared to other packages.

Test under _2.3 GHz 8-Core Intel Core i9_ under <https://github.com/ringsaturn/tzf/releases/tag/v0.9.0>

```
tests.py ....                                                                                                                                                  [100%]


------------------------------------------------------------------------------------------------------ benchmark: 2 tests -----------------------------------------------------------------------------------------------------
Name (time in us)                Min                       Max                      Mean                 StdDev                    Median                    IQR            Outliers          OPS            Rounds  Iterations
-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
test_tzfpy_random            27.5630 (1.0)            140.6980 (1.0)             45.2745 (1.0)          17.8666 (1.0)             38.7595 (1.0)           5.1460 (1.0)         35;58  22,087.4695 (1.0)         282           1
test_iter_global      2,348,899.0950 (>1000.0)  2,435,324.4310 (>1000.0)  2,396,214.2868 (>1000.0)  38,911.0747 (>1000.0)  2,394,437.6630 (>1000.0)  71,749.6862 (>1000.0)       2;0       0.4173 (0.00)          5           1
-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------

Legend:
  Outliers: 1 Standard Deviation from Mean; 1.5 IQR (InterQuartile Range) from 1st Quartile and 3rd Quartile.
  OPS: Operations Per Second, computed as 1 / Mean
```

`tzfpy` use about 130MB memory because `tzf` store all polygon in memory.

```bash
fil-profile run tzfpy/tests.py
```
