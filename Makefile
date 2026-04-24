# kinax-go — build the ObjC companion dylib (universal arm64+x86_64),
# then Go tests & CLI.
#
# Requirements:
#   - macOS 12+ (Accessibility API is stable across versions)
#   - Xcode Command Line Tools (for clang + frameworks)
#   - Go 1.22+

EMBEDDED_DYLIB := internal/dylib/libkinax_sync.dylib
OBJC_SRC       := objc/kinax_ax.m

ARCHES  ?= arm64 x86_64
ARCH_FLAGS := $(foreach a,$(ARCHES),-arch $(a))

CLANG_FLAGS := -dynamiclib -fobjc-arc -O2
FRAMEWORKS  := -framework Foundation \
               -framework CoreGraphics \
               -framework ApplicationServices \
               -framework AppKit

.PHONY: all dylib build test vet lint clean cli install-cli help

all: dylib build

$(EMBEDDED_DYLIB): $(OBJC_SRC)
	@echo "→ Building universal dylib ($(ARCHES))"
	@mkdir -p $(@D)
	clang $(CLANG_FLAGS) $(ARCH_FLAGS) $(OBJC_SRC) $(FRAMEWORKS) -o $@
	@echo "→ Verifying architectures"
	@file $@

dylib: $(EMBEDDED_DYLIB)

build: dylib
	go build ./...

test: dylib
	go test ./...

test-integration: dylib
	go test -tags integration ./...

vet:
	go vet ./...

lint: vet
	@command -v staticcheck >/dev/null 2>&1 && staticcheck ./... || echo "skip staticcheck"
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || echo "skip golangci-lint"

cli: dylib
	go build -o kinax ./cmd/kinax
	@echo "→ built ./kinax ($(shell du -h kinax | cut -f1))"

install-cli: dylib
	go install ./cmd/kinax

clean:
	rm -f libkinax_sync.dylib kinax
	rm -rf ~/Library/Caches/kinax-go
	go clean ./...

clean-all: clean
	rm -f $(EMBEDDED_DYLIB)

help:
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?##"}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'
