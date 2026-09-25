NAME=go-errors

.PHONY: fmt vet lint test cover bench fuzz tags

fmt:
	gofmt -s -w .

vet:
	go vet ./...

lint:
	golangci-lint run ./...

test:
	go test -race -count=1 ./...

cover:
	go test -race -coverprofile=coverage.out ./... && go tool cover -func=coverage.out

bench:
	go test -run=^$$ -bench=. -benchmem ./...

# FUZZTIME=5m make fuzz
FUZZTIME ?= 60s
fuzz:
	go test -run=^$$ -fuzz=FuzzErrorTrees -fuzztime=$(FUZZTIME) .
	go test -run=^$$ -fuzz=FuzzJoinAppendFlatten -fuzztime=$(FUZZTIME) .

tags: test
	@bash -c ' \
		version=$$(cat "$(CURDIR)/version" 2>/dev/null || echo "0.0.0") && \
		tag=v$$version && \
		echo "→ tag: $$tag" && \
		if [[ ! $$(git tag -l "$$tag") ]]; then \
			git tag -a "$$tag" -m "Release $$version" && \
			git push origin "$$tag" -o ci.skip && \
			echo "✅ Tagged and pushed $$tag"; \
		else \
			echo "⚠️  Tag $$tag already exists"; \
		fi \
	'
