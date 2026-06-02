
.DEFAULT_GOAL := help

help:
	@echo "Vit Makefile - Available recipes:"
	@echo ""
	@echo "  Development:"
	@echo "    make build             - build (portable+minimal)"
	@echo ""
	@echo "  Vendored Binaries (optional, requires internet):"
	@echo "    make fetch-vendor      - Download fzf binaries for all platforms"
	@echo ""

build:
	@cd scripts/build && go build -o /tmp/vit-build . && cd ../.. && /tmp/vit-build build both

fetch-vendor:
	@cd scripts/build && go build -o /tmp/vit-build . && cd ../.. && /tmp/vit-build fetch-vendor
