.PHONY: dev dev-frontend dev-backend build build-frontend test lint clean

GO?=go
GO_BUILD_TAGS?=goolm,stdjson
GOFLAGS?=-tags $(GO_BUILD_TAGS)
CGO_ENABLED?=0
export CGO_ENABLED

FRONTEND_DIR=frontend
BACKEND_DIR=backend
EXT=
ifeq ($(OS),Windows_NT)
	EXT=.exe
endif
CLAWY_BINARY?=$(abspath ../../build/clawy$(EXT))

# Both development and production use the same Clawy executable.
build:
	@bash ../../scripts/build-native.sh

build-frontend:
	@bash ../../scripts/build-web.sh

dev: build
	@$(MAKE) dev-backend BACKEND_ARGS='--no-browser' & $(MAKE) dev-frontend

dev-frontend:
	cd $(FRONTEND_DIR) && pnpm dev

dev-backend:
	"$(CLAWY_BINARY)" start $(BACKEND_ARGS)

test:
	$(GO) test $(GOFLAGS) ./backend/...
	cd $(FRONTEND_DIR) && pnpm lint

lint:
	$(GO) vet $(GOFLAGS) ./backend/...
	cd $(FRONTEND_DIR) && pnpm format && pnpm lint

clean:
	@node -e "const fs=require('fs'); for (const p of ['frontend/dist','backend/dist']) fs.rmSync(p,{recursive:true,force:true})"
	@node frontend/scripts/ensure-backend-gitkeep.cjs
