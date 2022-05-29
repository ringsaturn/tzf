# TZF: a timezone finder for Go.

```mermaid
graph TD
    C[H3 Based Approximation file]
    D[Probuf based Bin file]
    H[Polygon search component]
    D --> |Reduce via cmd/reducePolygon|D
    A[Raw timezone boundary JSON file] --> |Convert via cmd/tzjson2pb|D
    D --> |Uber H3 Polyfill|C
    D --> H
    C --> GetTimezone
    H --> GetTimezone
```

## TODO

- [x] POC: polygon search based
- [x] Reduce Polygon size option
  - [x] Reduce float precise
  - [x] Reduce line numbers
- [ ] H3 Based Approximation, something like Placekey

## Thanks

- <https://github.com/tidwall/geometry>
- <https://github.com/jannikmi/timezonefinder>
- <https://github.com/evansiroky/timezone-boundary-builder>
