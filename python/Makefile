.PHONY:build
build:
	cd tzfpy;go build -o tzf.so -buildmode=c-shared main.go

build-macos:
	cd tzfpy;go build -o tzf.cpython-38-darwin.so -buildmode=c-shared main.go

build-linux:
	cd tzfpy;go build -o tzf.cpython-36m-x86_64-linux-gnu.so -buildmode=c-shared main.go

pkg:
	make build
	poetry build
