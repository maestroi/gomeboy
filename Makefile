# Common local commands for gomeboy.
# Override ROM / extra flags:  make run ROM=tetris.gb ARGS='-driver glfw -scale 4'

GO      ?= go
NPM     ?= npm
WEB_UI  := pkg/display/web/gomeboy-web
IMAGE   ?= gomeboy-web:latest
ROM     ?= game.gb
ARGS    ?=

.DEFAULT_GOAL := help

.PHONY: all build desktop web agent frontend test vet regressions check \
	run run-web run-agent docker clean help

all: build

build: desktop web agent

desktop:
	$(GO) build -o gomeboy .

web:
	$(GO) build -o gomeboy-web ./cmd/gomeboy-web

agent:
	$(GO) build -o gomeboy-agent ./cmd/gomeboy-agent

frontend:
	$(NPM) run build --prefix $(WEB_UI)

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

regressions:
	$(GO) test -tags test -v ./tests -run Test_Regressions

check: vet test

run: desktop
	./gomeboy -rom "$(ROM)" $(ARGS)

run-web: web
	./gomeboy-web -rom "$(ROM)" $(ARGS)

run-agent: agent
	./gomeboy-agent -rom "$(ROM)" $(ARGS)

docker:
	docker build -t $(IMAGE) .

clean:
	rm -f gomeboy gomeboy-web gomeboy-agent

help:
	@echo "gomeboy"
	@echo "  make build         desktop + web + agent binaries"
	@echo "  make desktop       ./gomeboy"
	@echo "  make web           ./gomeboy-web"
	@echo "  make agent         ./gomeboy-agent"
	@echo "  make frontend      Svelte UI (npm run build)"
	@echo "  make test          go test ./..."
	@echo "  make vet           go vet ./..."
	@echo "  make check         vet + test"
	@echo "  make regressions   ROM regression suite (needs tests/roms/)"
	@echo "  make run           ./gomeboy -rom \$$ROM \$$ARGS"
	@echo "  make run-web       web player on :8090"
	@echo "  make run-agent     agent spectator on :8090"
	@echo "  make docker        docker build -t $(IMAGE)"
	@echo "  make clean         remove binaries"
	@echo
	@echo "  ROM=$(ROM)  ARGS=$(ARGS)"
