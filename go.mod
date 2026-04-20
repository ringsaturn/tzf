module github.com/ringsaturn/tzf

go 1.25.0

require (
	github.com/loov/hrtime v1.0.4
	github.com/mitchellh/mapstructure v1.5.0
	github.com/paulmach/orb v0.13.0
	github.com/ringsaturn/go-cities.json v0.6.13
	github.com/ringsaturn/polyf v0.2.2
	github.com/ringsaturn/tzf-dist v0.0.0
	github.com/tidwall/geojson v1.4.6
	github.com/tidwall/lotsa v1.0.5
	github.com/tidwall/rtree v1.10.0
	github.com/twpayne/go-polyline v1.1.1
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/tidwall/geoindex v1.7.0 // indirect
	go.mongodb.org/mongo-driver/v2 v2.5.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/ringsaturn/tzf-dist => ../tzf-dist
