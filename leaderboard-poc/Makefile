.PHONY: run-api run-aggregator run-historywriter run-notifier seed build

run-api:
	go run ./cmd/api

run-aggregator:
	go run ./cmd/aggregator

run-historywriter:
	go run ./cmd/historywriter

run-notifier:
	go run ./cmd/notifier

seed:
	go run ./cmd/seed --users 20 --events 200

build:
	go build ./...
