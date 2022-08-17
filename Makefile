PROTO_FILES=$(shell find pb -name *.proto)

install:
	go mod download
	go install github.com/mfridman/tparse@latest
	go install github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc@latest

build:
	cd cmd/reducePolygon;go build
	cd cmd/tzjson2pb;go build

fmt:
	find pb/ -iname *.proto | xargs clang-format -i --style=Google
	go fmt ./...


.PHONY:pb
pb:
	protoc  --proto_path=. \
			--doc_out=. --doc_opt=html,index.html,source_relative \
			--go_out=paths=source_relative:. \
			$(PROTO_FILES)

test:
	golangci-lint run ./...
	go test -json -race ./... -v -coverprofile=coverage.out  | tparse -all

cover: test
	go tool cover -html=coverage.out -o=coverage.html

compare_lite_full:
	go run cmd/compare-lite-full/main.go

bench:
	go test -bench=. ./...
