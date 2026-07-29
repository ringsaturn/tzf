module github.com/ringsaturn/tzf

go 1.25.0

require (
	github.com/ringsaturn/go-cities.json v0.6.13
	github.com/ringsaturn/orb v0.14.1-0.20260729042145-b067a66a7f4b
	github.com/ringsaturn/tzf-dist v0.0.2026-c-fix1
	github.com/tidwall/rtree v1.10.0
	golang.org/x/sync v0.21.0
	google.golang.org/protobuf v1.36.11
)

require github.com/tidwall/geoindex v1.7.0 // indirect

retract v1.2.4 // requires the nonexistent github.com/paulmach/orb v0.14.0
