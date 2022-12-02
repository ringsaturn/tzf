PROTO_FILES=$(shell find pb -name *.proto)

install:
	go mod download
	go install github.com/golang/protobuf/protoc-gen-go@latest
	go install github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc@latest

fmt:
	find pb/ -iname *.proto | xargs clang-format -i --style=Google
	go fmt ./...

.PHONY:pb
pb:
	protoc  --proto_path=. \
			--doc_out=. --doc_opt=html,pb.html,source_relative \
			--go_out=paths=source_relative:. \
			$(PROTO_FILES)

test:
	golangci-lint run ./...
	go test -v -coverprofile=coverage.out ./...

cover: test
	go tool cover -html=coverage.out -o=coverage.html

comparetzpb_gen:
	go run cmd/comparetzpb/main.go

bench:
	go test -bench=. ./...
