# TZF: a timezone finder for Go.

```mermaid
graph TD
    D[Probuf based Bin file]
    H[Polygon search component]
    D --> |Reduce via cmd/reducePolygon|D
    A[Raw timezone boundary JSON file] --> |Convert via cmd/tzjson2pb|D
    D --> H
    H --> GetTimezone
```

## Quick Start

```go
package main

import (
	"fmt"

	"github.com/ringsaturn/tzf"
	tzfrel "github.com/ringsaturn/tzf-rel"
	"github.com/ringsaturn/tzf/pb"
	"google.golang.org/protobuf/proto"
)

func main() {
	input := &pb.Timezones{}

	// Lite data, about 16.7MB
	dataFile := tzfrel.LiteData

	// Full data, about 83.5MB
	// dataFile := tzfrel.FullData

	if err := proto.Unmarshal(dataFile, input); err != nil {
		panic(err)
	}
	finder, _ := tzf.NewFinderFromPB(input)
	fmt.Println(finder.GetTimezoneName(111.8674, 34.4200))
	fmt.Println(finder.GetTimezoneName(-97.8674, 34.4200))
	fmt.Println(finder.GetTimezoneName(121.3547, 31.1139))
	fmt.Println(finder.GetTimezoneName(139.4382, 36.4432))
	fmt.Println(finder.GetTimezoneName(24.5212, 50.2506))
	fmt.Println(finder.GetTimezoneName(-0.9671, 52.0152))
	fmt.Println(finder.GetTimezoneName(-4.5706, 46.2747))
	fmt.Println(finder.GetTimezoneName(111.9781, 45.0182))
	fmt.Println(finder.GetTimezoneName(-73.7729, 38.3530))
}
```

Output:

```
Asia/Shanghai
America/Chicago
Asia/Shanghai
Asia/Tokyo
Europe/Kiev
Europe/London
Etc/GMT
Asia/Shanghai
Etc/GMT+5
```

## Data

Original data download from <https://github.com/evansiroky/timezone-boundary-builder>.

Preprocessed probuf data can get from <https://github.com/ringsaturn/tzf-rel> which has Go's `embed` support.

### Which dataset should I use

The lite data(16.7MB) doesn't work well in oceans but works well in most lands.

The full data(83.5MB) could work anywhere but requires more memory usage.

You can see ranges that results diff in this [gist][points_not_equal].

[points_not_equal]: https://gist.github.com/ringsaturn/8e1614146127cb25bf4d1325df347d22

## Related Links

- <https://ringsaturn.github.io/tzf/>: Continuous Benchmark Result
  - <https://ringsaturn.github.io/tz-benchmark/> Continuous Benchmark Compared with [`evanoberholster/timezoneLookup`](https://github.com/evanoberholster/timezoneLookup) Result
- <https://github.com/ringsaturn/tzf-rel>: Preprocessed probuf data release repo
- <https://github.com/ringsaturn/tzf-server>: HTTP Server for debug

## Thanks

- <https://github.com/tidwall/geojson>
- <https://github.com/jannikmi/timezonefinder>
- <https://github.com/evansiroky/timezone-boundary-builder>
