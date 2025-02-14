package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"

	tzfrellite "github.com/ringsaturn/tzf-rel-lite"
	pb "github.com/ringsaturn/tzf/gen/go/pb/v1"
	"google.golang.org/protobuf/proto"
)

const API = "https://api.github.com/repos/evansiroky/timezone-boundary-builder/tags"

type TagsResponseItem struct {
	Name   string `json:"name"`
	Commit Commit `json:"commit"`
	// ZipballURL string `json:"zipball_url"`
	// TarballURL string `json:"tarball_url"`
	// NodeID     string `json:"node_id"`
}

type Commit struct {
	Sha string `json:"sha"`
	URL string `json:"url"`
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	verbose := flag.Bool("verbose", false, "show more logs")
	flag.Parse()
	ctx := context.Background()

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, API, nil)
	must(err)

	httpResponse, err := http.DefaultClient.Do(httpRequest)
	must(err)
	defer httpResponse.Body.Close()

	resp := []*TagsResponseItem{}
	err = json.NewDecoder(httpResponse.Body).Decode(&resp)
	must(err)

	latestTag := resp[0].Name

	input := &pb.PreindexTimezones{}
	if err := proto.Unmarshal(tzfrellite.PreindexData, input); err != nil {
		panic(err)
	}
	if *verbose {
		log.Printf("input.Version=%v, latestTag=%v\n", input.Version, latestTag)
	}
	if input.Version == latestTag {
		log.Println("Same version, bye!")
		return
	}
	fmt.Printf("TIMEZONE_BOUNDARY_VERSION=%s\n", latestTag)
}
