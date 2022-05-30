// Compress provide read&write with data compressed by slimarray https://github.com/openacid/slimarray
package compress

import (
	"github.com/openacid/slimarray"
	"github.com/ringsaturn/tzf/pb"
)

const Times int32 = 10000

func FromNormalPB(input *pb.Timezones) *pb.CompressTimezones {
	out := &pb.CompressTimezones{
		Precise: Times,
	}
	for _, timezone := range input.Timezones {
		newCompressTimezone := &pb.CompressTimezone{
			Name:             timezone.Name,
			CompressPolygons: make([]*pb.CompressPolygon, 0),
		}
		for _, polygon := range timezone.Polygons {
			lngs := make([]uint32, 0)
			lats := make([]uint32, 0)

			for _, point := range polygon.Points {
				lngs = append(lngs, uint32(point.Lng*10000))
				lats = append(lats, uint32(point.Lng*10000))
			}

			newCompressPolygon := &pb.CompressPolygon{
				LngSlimArray: slimarray.NewU32(lngs),
				LatSlimArray: slimarray.NewU32(lats),
			}
			newCompressTimezone.CompressPolygons = append(newCompressTimezone.CompressPolygons, newCompressPolygon)
		}
		out.CompressTimezones = append(out.CompressTimezones, newCompressTimezone)
	}
	return out
}

func ToNormalPB(input *pb.CompressTimezones) *pb.Timezones {
	out := &pb.Timezones{}
	floatK := float32(input.Precise)
	for _, compresstimezone := range input.CompressTimezones {
		timezone := &pb.Timezone{
			Name:     compresstimezone.Name,
			Polygons: make([]*pb.Polygon, 0),
		}
		for _, compressPoly := range compresstimezone.CompressPolygons {
			poly := &pb.Polygon{
				Points: make([]*pb.Point, 0),
			}
			for i := 0; i < int(compressPoly.LngSlimArray.N); i++ {
				poly.Points = append(poly.Points, &pb.Point{
					Lng: float32(compressPoly.LngSlimArray.Get(int32(i))) / floatK,
					Lat: float32(compressPoly.LatSlimArray.Get(int32(i))) / floatK,
				})
			}
			timezone.Polygons = append(timezone.Polygons, poly)
		}
		out.Timezones = append(out.Timezones, timezone)
	}
	return out
}
