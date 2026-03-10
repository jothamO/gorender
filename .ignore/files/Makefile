.PHONY: build test test-integration lint clean run-server

# Build both binaries
build:
	go build -o bin/gorender ./cmd/gorender
	go build -o bin/gorendersd ./cmd/gorendersd

# Unit tests only (no Chrome/ffmpeg needed)
test:
	go test ./internal/pipeline/... ./internal/scheduler/... ./internal/jobs/... ./internal/server/... -v -race

# Integration tests (needs Chrome, ffmpeg, and GORENDER_TEST_URL)
test-integration:
	GORENDER_TEST_URL=$(GORENDER_TEST_URL) \
	go test ./internal/integration/... -tags=integration -v -timeout 120s

# Run the render server with sensible dev defaults
run-server:
	go run ./cmd/gorendersd \
		--addr :8080 \
		--max-jobs 1 \
		--workers 2 \
		--output-dir ./output \
		--verbose

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/ output/

# Quick smoke test against a local composition
smoke:
	./bin/gorender render \
		--url http://localhost:3000/story/test \
		--frames 30 \
		--fps 30 \
		--out /tmp/smoke-test.mp4 \
		--workers 2 \
		--verbose
