.PHONY:build
build:
	cd tzfpy;go build -o tzf.so -buildmode=c-shared main.go

pkg:
	make build
	poetry build
