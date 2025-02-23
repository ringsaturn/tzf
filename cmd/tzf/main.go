// tzf-cli tool for local query.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ringsaturn/tzf"
	tzfrellite "github.com/ringsaturn/tzf-rel-lite"
	pb "github.com/ringsaturn/tzf/gen/go/tzf/v1"
	"google.golang.org/protobuf/proto"
)

var finder tzf.F

func init() {
	input := &pb.CompressedTimezones{}
	dataFile := tzfrellite.LiteCompressData
	err := proto.Unmarshal(dataFile, input)
	if err != nil {
		panic(err)
	}
	finder, err = tzf.NewFinderFromCompressed(input)
	if err != nil {
		panic(err)
	}
}

type StdinOrder int

const (
	LngLat StdinOrder = iota
	LatLng
)

func main() {
	var lng float64
	var lat float64
	var stdinOrder StdinOrder
	flag.Float64Var(&lng, "lng", 0.0, "Longitude")
	flag.Float64Var(&lat, "lat", 0.0, "Latitude")
	flag.Func("stdin-order", "Read multiple coordinates from stdin in given order", func(s string) error {
		if s == "lng-lat" || s == "lon-lat" {
			stdinOrder = LngLat
		} else if s == "lat-lng" || s == "lat-lon" {
			stdinOrder = LatLng
		} else {
			return errors.New("invalid order, must be one of lng-lat, lat-lng")
		}
		return nil
	})
	flag.Parse()
	hasLng := false
	hasLat := false
	hasStdinOrder := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "lng" {
			hasLng = true
		}
		if f.Name == "lat" {
			hasLat = true
		}
		if f.Name == "stdin-order" {
			hasStdinOrder = true
		}
	})
	if hasLng != hasLat {
		fmt.Fprintln(os.Stderr, "Both -lat and -lng must be passed")
		os.Exit(2)
	}
	if (hasLng || hasLat) == hasStdinOrder {
		fmt.Fprintln(os.Stderr, "Either -{lat/lng} or -stdin-order must be passed")
		os.Exit(2)
	}
	if hasLng || hasLat {
		fmt.Println(finder.GetTimezoneName(lng, lat))
		return
	}
	if hasStdinOrder {
		scanner := bufio.NewScanner(bufio.NewReader(os.Stdin))
		for scanner.Scan() {
			line := strings.FieldsFunc(scanner.Text(), func(c rune) bool { return c == ' ' || c == '\t' || c == ',' || c == ';' })
			if len(line) != 2 {
				panic("Line does not contain two coordinates")
			}
			a, err := strconv.ParseFloat(line[0], 64)
			if err != nil {
				panic(err)
			}
			b, err := strconv.ParseFloat(line[1], 64)
			if err != nil {
				panic(err)
			}
			lng, lat := a, b
			if stdinOrder == LatLng {
				lng, lat = b, a
			}
			fmt.Println(finder.GetTimezoneName(lng, lat))
		}
	}
}
