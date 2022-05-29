// CLI tool to reduce polygon filesize
package main

import (
	"io/ioutil"
	"math"
	"os"
	"strings"

	"github.com/ringsaturn/tzf/pb"
	"google.golang.org/protobuf/proto"
)

const (
	Skip           int     = 5     // At least skip how many point
	Precise        float64 = 10000 // round float precise
	MIN_POINT_DIST float64 = 10    // min dist to previous point, except begin&end point
	EARTH_RADIUS           = 6371  // km, for distance computing
)

func radians(degree float64) float64 {
	return degree * math.Pi / 180
}

func GeoDistance(lat1, long1, lat2, long2 float64) float64 {
	lat1 = radians(lat1)
	long1 = radians(long1)
	lat2 = radians(lat2)
	long2 = radians(long2)

	hav_lat := 0.5 * (1 - math.Cos(lat1-lat2))
	hav_long := 0.5 * (1 - math.Cos(long1-long2))
	radical := math.Sqrt(hav_lat + math.Cos(lat1)*math.Cos(lat2)*hav_long)
	return 2 * EARTH_RADIUS * math.Asin(radical) * 1000
}

func Reduce(input *pb.Timezones) *pb.Timezones {
	output := &pb.Timezones{}
	for _, timezone := range input.Timezones {
		reducedTimezone := &pb.Timezone{
			Name: timezone.Name,
		}
		for _, polygon := range timezone.Polygons {
			newPoly := &pb.Polygon{}
			maxIndex := len(polygon.Points) - 1
			for index, point := range polygon.Points {
				if index == maxIndex || index == 0 {
					newPoly.Points = append(newPoly.Points, &pb.Point{
						Lng: float32(math.Round(float64(point.Lng)*Precise) / Precise),
						Lat: float32(math.Round(float64(point.Lat)*Precise) / Precise),
					})
					continue
				}
				if index%Skip != 0 {
					continue
				}
				previousPoint := newPoly.Points[len(newPoly.Points)-1]
				dist := GeoDistance(float64(point.Lat), float64(point.Lng), float64(previousPoint.Lat), float64(previousPoint.Lng))
				if dist < MIN_POINT_DIST {
					continue
				}
				newPoly.Points = append(newPoly.Points, &pb.Point{
					Lng: float32(math.Round(float64(point.Lng)*Precise) / Precise),
					Lat: float32(math.Round(float64(point.Lat)*Precise) / Precise),
				})
			}
			reducedTimezone.Polygons = append(reducedTimezone.Polygons, newPoly)
		}
		output.Timezones = append(output.Timezones, reducedTimezone)
	}
	return output
}

func main() {
	originalProbufPath := os.Args[1]
	rawFile, err := ioutil.ReadFile(originalProbufPath)
	if err != nil {
		panic(err)
	}
	input := &pb.Timezones{}
	if err := proto.Unmarshal(rawFile, input); err != nil {
		panic(err)
	}
	output := Reduce(input)

	outputPath := strings.Replace(originalProbufPath, ".pb", ".reduce.pb", 1)
	outputBin, _ := proto.Marshal(output)
	f, err := os.Create(outputPath)
	if err != nil {
		panic(err)
	}
	_, _ = f.Write(outputBin)

	// For debug
	// func() {
	// 	outputJSONB, _ := json.MarshalIndent(output, "", "  ")
	// 	f, err := os.Create(strings.Replace(outputPath, ".pb", ".pb.json", 1))
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// 	f.Write(outputJSONB)
	// }()
}
