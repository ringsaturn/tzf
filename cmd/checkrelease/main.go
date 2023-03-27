package main

import (
	"context"
	"net/http"
	"os"

	"github.com/ringsaturn/requests"
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
	ctx := context.Background()
	resp := []*TagsResponseItem{}
	err := requests.ReqWithExpectJSONResponse(ctx, http.DefaultClient, "GET", API, nil, &resp)
	if err != nil {
		panic(err)
	}
	latestTag := resp[0].Name
	os.Setenv("TIMEZONE_BOUNDARY_VERSION", latestTag)
}
