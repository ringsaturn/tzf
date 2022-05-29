PROTO_FILES=$(shell find pb -name *.proto)

fmt:
	find pb/ -iname *.proto | xargs clang-format -i --style=Google
	go fmt ./...


.PHONY:pb
pb:
	protoc  --proto_path=. \
			--go_out=paths=source_relative:. \
			--go-errors_out=paths=source_relative:. \
			$(PROTO_FILES)
