.PHONY: build test test-integration lint clean run-server scaffold-vanilla scaffold-svelte scaffold-react

build:
	go build -o bin/gorender ./cmd/gorender
	go build -o bin/gorendersd ./cmd/gorendersd

test:
	go test ./internal/pipeline/... ./internal/scheduler/... ./internal/jobs/... ./internal/server/... -v -race

test-integration:
	GORENDER_TEST_URL=$(GORENDER_TEST_URL) \
	go test ./internal/integration/... -tags=integration -v -timeout 120s

run-server:
	go run ./cmd/gorendersd \
		--addr :8080 \
		--max-jobs 1 \
		--workers 2 \
		--output-dir ./output \
		--verbose

# Scaffold examples (for local testing)
scaffold-vanilla:
	./bin/gorender new test-vanilla --template vanilla

scaffold-svelte:
	./bin/gorender new test-svelte --template svelte --fps 24 --frames 720

scaffold-react:
	./bin/gorender new test-react --template react

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/ output/ test-vanilla/ test-svelte/ test-react/

smoke:
	./bin/gorender render \
		--url http://localhost:3000/story/test \
		--frames 30 --fps 30 \
		--out /tmp/smoke-test.mp4 \
		--workers 2 --verbose

