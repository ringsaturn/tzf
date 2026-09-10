# Single module: github.com/ringsaturn/tzf/v2 (the v1 line lives on its own
# branch/tags). The runtime embeds come from the published tzf-dist module;
# the parity targets need ./scripts/build-tzf-dist-dev.sh once after cloning —
# it runs the full pipeline from the upstream raw GeoJSON, keeps the gob
# intermediates in tmp/tzf-dist-dev as parity fixtures, and installs the
# artifacts into the sibling ../tzf-dist checkout for its embed tests.

DEV_DIR := tmp/tzf-dist-dev
DIST_DIR := ../tzf-dist
PARITY_ENV := TZF_PARITY_TOPO=$(abspath $(DEV_DIR)/combined-with-oceans.topology.compress.topo.gob) \
	TZF_PARITY_PREINDEX=$(abspath $(DEV_DIR)/combined-with-oceans.topology.preindex.gob)

fmt:
	gofmt -w -l .

test:
	golangci-lint run ./...
	$(PARITY_ENV) go test -race ./...

cover:
	$(PARITY_ENV) go test -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html

bench:
	go test -bench=. -benchmem -run=NoTests . | tee benchmark_result.txt

bench-memory:
	go run ./internal/cmd/bench-memory | tee memory_result.txt

parity:
	go run ./internal/cmd/embedcompare \
		-preindex $(DEV_DIR)/combined-with-oceans.topology.preindex.gob \
		-tzm $(DIST_DIR)/lite.tzm \
		$(DEV_DIR)/combined-with-oceans.topology.compress.topo.gob $(DIST_DIR)/lite.tzb

.PHONY: fmt test cover bench bench-memory parity
