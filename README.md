# TZF: a timezone finder for Go.

```mermaid
graph TD
    C[H3 Based Approximation file]
    D[Probuf based Bin file]
    H[Polygon search component]
    D --> |Reduce resolution and precise|D
    A[Raw timezone boundary JSON file] --> |Convert|D
    D --> |Uber H3 Polyfill|C
    D --> H
    C --> GetTimezone
    H --> GetTimezone
```

TODO:

- [ ] POC: polygon search based
- [ ] Reduce Polygon size option
  - [ ] Reduce float precise
  - [ ] Reduce line numbers
- [ ] H3 Based Approximation, something like Placekey
