
.DEFAULT_GOAL := help

help:
	@echo "Vit Makefile - Available recipes:"
	@echo ""
	@echo "  Development:"
	@echo "    make build             - build (portable+minimal)"
	@echo "    make test              - Run all tests"
	@echo "    make vet               - Run go vet (static checks)"
	@echo ""
	@echo "  Vendored Binaries (optional, requires internet):"
	@echo "    make fetch-vendor      - Download fzf binaries for all platforms"
	@echo ""
	@echo "  Misc:"
	@echo "    make cloc              - Count lines of code (requires cloc)"
	@echo "    make todo              - List TODO/FIXME comments grouped by file:func"

build: todo
	@cd scripts/build && go build -o /tmp/vit-build . && cd ../.. && /tmp/vit-build build both

test:
	go test -C src -v ./...

vet:
	go vet -C src ./...

fetch-vendor:
	@cd scripts/build && go build -o /tmp/vit-build . && cd ../.. && /tmp/vit-build fetch-vendor

cloc:
	cloc src --match-f='.*\.go$$' --not-match-f='.*_test\.go$$'

todo:
	@cd scripts/todo && go build -o /tmp/vit-todo . && cd ../.. && /tmp/vit-todo src
