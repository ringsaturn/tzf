.PHONY:build
build:
	cd tzfpy;CGO_ENABLED=1 go build -o tzf.so -buildmode=c-shared main.go

pkg:
	make build
	poetry build
