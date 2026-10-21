.PHONY: help build test race run run-all fmt

DAY ?= day01_worker_pool

help:
	@printf '%s\n' \
		'make build                 编译全部章节' \
		'make test                  运行全部测试' \
		'make race                  运行数据竞争检测' \
		'make run DAY=day15_capstone 执行指定章节' \
		'make run-all               依次执行 15 个 main' \
		'make fmt                   格式化代码'

build:
	go build ./...

test:
	go test ./...

race:
	go test -race ./...

run:
	go run ./$(DAY)

run-all:
	@set -e; for d in day*/; do \
		echo "=== $${d%/} ==="; \
		go run "./$${d%/}"; \
	done

fmt:
	gofmt -w .
