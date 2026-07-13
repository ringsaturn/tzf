## Evaluation result

- Input: `combined-with-oceans.dist.bin`
- Dataset version: `2026c`
- Douglas-Peucker epsilon: `0.001000 degrees`
- Unique source arcs: `532881`
- Changed arcs: `402246`
- Original unique boundary length: `1309954.762 km`
- Changed boundary length: `727904.587 km`
- Error strip area: `16828.273416 km2`
- Maximum single strip area: `636.827392 km2`
- Runtime: `17.387s`

### Boundary displacement

| Metric | Distance |
|---|---:|
| Length-weighted p50 | 1.300 m |
| Length-weighted p95 | 66.400 m |
| Length-weighted p99 | 92.100 m |
| Length-weighted p99.9 | 106.500 m |
| Certified maximum | 111.232 m |
| Certification upper tolerance | +1.000 m |

Maximum location: `-9.5089798, 51.2220001`, timezone pair: `Etc/GMT+1` / `Europe/Dublin`.

| Threshold | Boundary length above threshold |
|---|---:|
| 10 m | 38.645884% |
| 50 m | 9.887938% |
| 100 m | 0.410467% |
| 500 m | 0.000000% |

### Error strip width

| Metric | Width |
|---|---:|
| Area-weighted p50 | 30.800 m |
| Area-weighted p95 | 480.900 m |
| Area-weighted p99 | 3796.100 m |
| Area-weighted p99.9 | 4530.600 m |

- Error area within 10 m of source boundary: `14.803018%`
- Error area within 50 m of source boundary: `74.803914%`
- Error area within 100 m of source boundary: `92.841622%`

### Largest timezone-pair error areas

| Timezone A | Timezone B | Area |
|---|---|---:|
| Africa/Algiers | Africa/Bamako | 650.930436 km2 |
| Asia/Tokyo | Etc/GMT-9 | 270.894129 km2 |
| Etc/GMT-2 | Europe/Athens | 246.578323 km2 |
| Australia/Brisbane | Etc/GMT-10 | 246.413524 km2 |
| Etc/GMT+10 | Pacific/Tahiti | 236.417040 km2 |
| Australia/Perth | Etc/GMT-8 | 187.601936 km2 |
| Asia/Shanghai | Etc/GMT-8 | 161.461702 km2 |
| America/Nome | Etc/GMT+11 | 150.519270 km2 |
| Etc/GMT-11 | Pacific/Majuro | 138.731181 km2 |
| Etc/GMT+9 | Pacific/Tahiti | 130.657100 km2 |
| America/Iqaluit | America/Toronto | 122.132326 km2 |
| Etc/GMT-11 | Pacific/Noumea | 118.142845 km2 |
| Etc/GMT-10 | Pacific/Chuuk | 116.621896 km2 |
| Etc/GMT+7 | Pacific/Easter | 115.526190 km2 |
| America/Santiago | Etc/GMT+5 | 109.451418 km2 |
| America/New_York | Etc/GMT+5 | 108.237312 km2 |
| Etc/GMT | Europe/London | 107.392607 km2 |
| Asia/Kolkata | Etc/GMT-5 | 101.524178 km2 |
| America/Cambridge_Bay | America/Rankin_Inlet | 99.355680 km2 |
| Etc/GMT-1 | Europe/Rome | 88.919766 km2 |

### Simplifier statistics

```text
topology_rings: total=2078 no_fixed=1480 one_fixed=36 multi_fixed=558 fallback=206
topology_points: input=8181689 snapped_inserted=14 fallback_points=10639 fixed_vertices=185696
topology_segments: total=187173 shared=4316(2.31%) skipped_short=182802(97.66%) cache_hits=2150 cache_misses=2166 cache_hit_rate=49.81%
topology_segment_points: input=8368850 output=1292272 reduction=84.56%
topology_segment_length_buckets: le10=183202 le25=336 le50=350 le100=453 gt100=2832
```
