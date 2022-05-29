# TZF: a timezone finder for Go.

```mermaid
graph TD
    B[Reduced JSON]
    C[H3 Based Approximation]
    D[Probuf Based]
    H[Polygon Search]
    RawJSON --> |Reduce|B
    RawJSON --> |Provider|D
    B --> |Provider|D
    D --> |Uber H3 Polyfill|C
    D --> |Point in Polygon Search Algo|H
    C --> GetTimezone
    H --> GetTimezone
```

TODO:

- [ ] POC: polygon search based
- [ ] Reduce Polygon size option
  - [ ] Reduce float precise
  - [ ] Reduce line numbers
- [ ] H3 Based Approximation, something like Placekey
