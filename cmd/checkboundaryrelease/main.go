package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/ringsaturn/requests"
	tzfrel "github.com/ringsaturn/tzf-rel"
	"github.com/ringsaturn/tzf/pb"
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

func main() {
	verbose := flag.Bool("verbose", false, "show more logs")
	flag.Parse()
	ctx := context.Background()
	resp := []*TagsResponseItem{}
	err := requests.ReqWithExpectJSONResponse(ctx, http.DefaultClient, "GET", API, nil, &resp)
	if err != nil {
		panic(err)
	}
	latestTag := resp[0].Name

	input := &pb.PreindexTimezones{}
	if err := proto.Unmarshal(tzfrel.PreindexData, input); err != nil {
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
