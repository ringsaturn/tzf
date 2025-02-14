package main

import (
	"os"

	"github.com/RoaringBitmap/roaring/roaring64"
	tzfrellite "github.com/ringsaturn/tzf-rel-lite"
	"github.com/ringsaturn/tzf/internal/pmtiles"
	"github.com/ringsaturn/tzf/pb"
	"google.golang.org/protobuf/proto"
)

func main() {
	input := &pb.PreindexTimezones{}
	if err := proto.Unmarshal(tzfrellite.PreindexData, input); err != nil {
		panic(err)
	}

	result := map[string]*roaring64.Bitmap{}

	for _, index := range input.Keys {
		name := index.Name
		if _, ok := result[name]; !ok {
			result[name] = roaring64.New()
		}
		id := pmtiles.ZxyToID(uint8(index.Z), uint32(index.X), uint32(index.Y))
		result[name].Add(id)

	}

	msgs := []*pb.PreindexBitmapsItem{}

	for name, bitmap := range result {
		bitmapBytes, err := bitmap.ToBytes()
		if err != nil {
			panic(err)
		}
		msgs = append(msgs, &pb.PreindexBitmapsItem{
			Name: name,
			Data: bitmapBytes,
		})
	}

	finalMsg := &pb.PreindexBitmaps{
		Items:   msgs,
		Version: input.Version,
	}

	finalBytes, err := proto.Marshal(finalMsg)
	if err != nil {
		panic(err)
	}

	// // compress final bytes
	// var b bytes.Buffer
	// w := gzip.NewWriter(&b)
	// w.Write(finalBytes)
	// w.Close()

	// finalOutput := b.Bytes()

	if err := os.WriteFile("preindex-bitmap.pb", finalBytes, 0644); err != nil {
		panic(err)
	}
}
