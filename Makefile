# Common local commands for gomeboy.
# Override ROM / extra flags:  make run ROM=tetris.gb ARGS='-driver glfw -scale 4'

GO      ?= go
ROM     ?= game.gb
ARGS    ?=

.DEFAULT_GOAL := help

.PHONY: all build desktop agent test vet regressions check \
	run run-agent clean help

all: build

build: desktop agent

desktop:
	$(GO) build -o gomeboy .

agent:
	$(GO) build -o gomeboy-agent ./cmd/gomeboy-agent

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

regressions:
	$(GO) test -tags test -v ./tests -run Test_Regressions

check: vet test

run: desktop
	./gomeboy -rom "$(ROM)" $(ARGS)

run-agent: agent
	./gomeboy-agent -rom "$(ROM)" $(ARGS)

clean:
	rm -f gomeboy gomeboy-agent

help:
	@echo "gomeboy"
	@echo "  make build         desktop + agent binaries"
	@echo "  make desktop       ./gomeboy"
	@echo "  make agent         ./gomeboy-agent"
	@echo "  make test          go test ./..."
	@echo "  make vet           go vet ./..."
	@echo "  make check         vet + test"
	@echo "  make regressions   ROM regression suite (needs tests/roms/)"
	@echo "  make run           ./gomeboy -rom \$$ROM \$$ARGS"
	@echo "  make run-agent     agent spectator on :8090"
	@echo "  make clean         remove binaries"
	@echo
	@echo "  ROM=$(ROM)  ARGS=$(ARGS)"
