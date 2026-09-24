## Evaluation result

- Source input: `combined-with-oceans.dist.gob`
- Candidate input: `generated with topology-aware simplification`
- Source dataset version: `2026c`
- Candidate dataset version: `2026c`
- Douglas-Peucker epsilon: `0.001000 degrees`
- Source points after topology normalization: `8183767`
- Candidate points: `1118325`
- Point reduction: `86.335%`
- Unique source arcs: `530665`
- Changed arcs: `399436`
- Original unique boundary length: `1298235.907 km`
- Changed boundary length: `716160.658 km`
- Error strip area: `16495.223666 km2`
- Maximum single strip area: `636.827392 km2`
- Junction vertices inserted by shared-edge deduplication (dropped before arc matching): `0`, maximum offset from the baseline ring: `0.000 m`
- Runtime: `18.383s`

### Boundary displacement

| Metric | Distance |
|---|---:|
| Length-weighted p50 | 1.100 m |
| Length-weighted p95 | 66.200 m |
| Length-weighted p99 | 92.000 m |
| Length-weighted p99.9 | 106.400 m |
| Certified maximum | 111.232 m |
| Certification upper tolerance | +1.000 m |

Maximum location: `-9.5089798, 51.2220001`, timezone pair: `Etc/GMT+1` / `Europe/Dublin`.

| Threshold | Boundary length above threshold |
|---|---:|
| 10 m | 38.308277% |
| 50 m | 9.785947% |
| 100 m | 0.406204% |
| 500 m | 0.000000% |

### Error strip width

| Metric | Width |
|---|---:|
| Area-weighted p50 | 30.800 m |
| Area-weighted p95 | 518.600 m |
| Area-weighted p99 | 3837.900 m |
| Area-weighted p99.9 | 4582.500 m |

- Error area within 10 m of source boundary: `14.813864%`
- Error area within 50 m of source boundary: `74.802919%`
- Error area within 100 m of source boundary: `92.769112%`

### Largest timezone-pair error areas

| Timezone A | Timezone B | Area |
|---|---|---:|
| Africa/Algiers | Africa/Bamako | 650.930436 km2 |
| Asia/Tokyo | Etc/GMT-9 | 276.119702 km2 |
| Etc/GMT-2 | Europe/Athens | 246.578323 km2 |
| Australia/Brisbane | Etc/GMT-10 | 246.413524 km2 |
| Etc/GMT+10 | Pacific/Tahiti | 240.570516 km2 |
| Australia/Perth | Etc/GMT-8 | 187.601936 km2 |
| Asia/Shanghai | Etc/GMT-8 | 162.984425 km2 |
| America/Nome | Etc/GMT+11 | 153.209446 km2 |
| Etc/GMT+9 | Pacific/Tahiti | 139.196927 km2 |
| Etc/GMT-11 | Pacific/Majuro | 138.731181 km2 |
| America/Iqaluit | America/Toronto | 122.132326 km2 |
| Etc/GMT-11 | Pacific/Noumea | 118.142845 km2 |
| Etc/GMT-10 | Pacific/Chuuk | 116.621896 km2 |
| Etc/GMT+7 | Pacific/Easter | 115.526190 km2 |
| America/Santiago | Etc/GMT+5 | 109.451418 km2 |
| Etc/GMT-12 | Pacific/Auckland | 109.420638 km2 |
| America/New_York | Etc/GMT+5 | 108.237312 km2 |
| Etc/GMT | Europe/London | 107.392607 km2 |
| Asia/Kolkata | Etc/GMT-5 | 101.524178 km2 |
| America/Cambridge_Bay | America/Rankin_Inlet | 99.355680 km2 |

### Simplifier statistics

```text
topology_rings: total=2078 no_fixed=1474 one_fixed=4 multi_fixed=596 fallback=60 hole_escape=0 resimplified=33
topology_points: input=8181689 snapped_inserted=14 fallback_points=6151 fixed_vertices=186348
topology_segments: total=187817 shared=4854(2.58%) skipped_short=183170(97.53%) skipped_small=94(0.05%) cache_hits=2404 cache_misses=2450 cache_hit_rate=49.53%
topology_segment_points: input=8369494 output=1298520 reduction=84.49%
topology_segment_length_buckets: le10=183679 le25=368 le50=371 le100=481 gt100=2918
```
