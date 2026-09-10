## Evaluation result

- Source input: `combined-with-oceans.json`
- Candidate input: `combined-with-oceans.topology.compress.topo.gob`
- Source dataset version: `not encoded`
- Candidate dataset version: `2026c`
- Source points after topology normalization: `8188413`
- Candidate points: `1117199`
- Point reduction: `86.356%`
- Unique source arcs: `533960`
- Changed arcs: `518450`
- Original unique boundary length: `1322519.809 km`
- Changed boundary length: `907611.201 km`
- Error strip area: `16962.270999 km2`
- Maximum single strip area: `636.860912 km2`
- Junction vertices inserted by shared-edge deduplication (dropped before arc matching): `9`, maximum offset from the baseline ring: `1.763 m`
- Runtime: `43.915s`

### Boundary displacement

| Metric | Distance |
|---|---:|
| Length-weighted p50 | 1.200 m |
| Length-weighted p95 | 66.400 m |
| Length-weighted p99 | 92.100 m |
| Length-weighted p99.9 | 106.600 m |
| Certified maximum | 111.701 m |
| Certification upper tolerance | +1.000 m |

Maximum location: `-67.4404221, 2.2153740`, timezone pair: `America/Bogota` / `America/Manaus`.

| Threshold | Boundary length above threshold |
|---|---:|
| 10 m | 38.493222% |
| 50 m | 9.855156% |
| 100 m | 0.413341% |
| 500 m | 0.000000% |

### Error strip width

| Metric | Width |
|---|---:|
| Area-weighted p50 | 30.900 m |
| Area-weighted p95 | 469.200 m |
| Area-weighted p99 | 3795.700 m |
| Area-weighted p99.9 | 4662.400 m |

- Error area within 10 m of source boundary: `14.760799%`
- Error area within 50 m of source boundary: `74.763160%`
- Error area within 100 m of source boundary: `92.829868%`

### Largest timezone-pair error areas

| Timezone A | Timezone B | Area |
|---|---|---:|
| Africa/Algiers | Africa/Bamako | 650.814924 km2 |
| Asia/Tokyo | Etc/GMT-9 | 268.254112 km2 |
| Australia/Brisbane | Etc/GMT-10 | 246.360231 km2 |
| Etc/GMT-2 | Europe/Athens | 245.000075 km2 |
| Etc/GMT+10 | Pacific/Tahiti | 234.829788 km2 |
| Australia/Perth | Etc/GMT-8 | 187.784112 km2 |
| Asia/Shanghai | Etc/GMT-8 | 160.750236 km2 |
| America/Nome | Etc/GMT+11 | 150.005663 km2 |
| Etc/GMT-11 | Pacific/Majuro | 139.623076 km2 |
| Etc/GMT+9 | Pacific/Tahiti | 125.714864 km2 |
| America/Iqaluit | America/Toronto | 122.479572 km2 |
| Etc/GMT-11 | Pacific/Noumea | 118.178278 km2 |
| Etc/GMT-10 | Pacific/Chuuk | 116.629052 km2 |
| Etc/GMT+7 | Pacific/Easter | 115.675541 km2 |
| America/Santiago | Etc/GMT+5 | 108.768766 km2 |
| America/New_York | Etc/GMT+5 | 108.576489 km2 |
| Etc/GMT | Europe/London | 107.996116 km2 |
| Asia/Kolkata | Etc/GMT-5 | 100.803076 km2 |
| America/Cambridge_Bay | America/Rankin_Inlet | 99.316964 km2 |
| Etc/GMT-1 | Europe/Rome | 90.675887 km2 |

