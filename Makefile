fmt:
	go fmt ./...
	buf format -w .

.PHONY:pb
pb:
	buf generate

test:
	golangci-lint run ./...
	go test -v -coverprofile=coverage.out ./...
	python3 -m unittest scripts/bench2summary_test.py

cover: test
	go tool cover -html=coverage.out -o=coverage.html

bench:
	go test -bench=. -benchmem -count=1 -timeout=600s .  | tee benchmark_result.txt

bench-memory:
	go run ./internal/cmd/bench-memory/... | tee memory_result.txt

bench-summary: bench bench-memory
	python3 scripts/bench2summary.py benchmark_result.txt memory_result.txt | tee bench_summary.txt

dep-licenses:
	rm -rf THIRD_PARTY_LICENSES
	go run github.com/google/go-licenses/v2@latest save ./... --save_path=THIRD_PARTY_LICENSES
	cp $$(go env GOPATH)/pkg/mod/github.com/ringsaturn/tzf-dist@$$(go list -m github.com/ringsaturn/tzf-dist | awk '{print $$2}')/LICENSE_DATA \
		THIRD_PARTY_LICENSES/github.com/ringsaturn/tzf-dist/LICENSE_DATA
	bash build_notice.sh

.PHONY: update-citation
update-citation:
	@set -eu; \
	tag="$$(git describe --tags --abbrev=0 HEAD)"; \
	commit="$$(git rev-list -n 1 "$$tag")"; \
	date="$$(git show -s --format=%cs "$$tag^{commit}")"; \
	tmp="$$(mktemp CITATION.cff.XXXXXX)"; \
	trap 'rm -f "$$tmp"' EXIT; \
	awk -v tag="$$tag" -v commit="$$commit" -v date="$$date" ' \
		BEGIN { updated = 0 } \
		/^commit:[[:space:]]*/ { print "commit: " commit; updated++; next } \
		/^version:[[:space:]]*/ { print "version: " tag; updated++; next } \
		/^date-released:[[:space:]]*/ { print "date-released: '\''" date "'\''"; updated++; next } \
		{ print } \
		END { if (updated != 3) exit 1 } \
	' CITATION.cff > "$$tmp"; \
	mv "$$tmp" CITATION.cff; \
	trap - EXIT; \
	printf 'Updated CITATION.cff to %s (%s, %s)\n' "$$tag" "$$commit" "$$date"
