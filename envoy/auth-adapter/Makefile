

cur := $(shell pwd)

gen-proto:
	@docker run --rm -v ${cur}:${cur} -w ${cur} znly/protoc:0.4.0 -I. ./auth.proto --go_out=plugins=grpc,import_path=main:.
