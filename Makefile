# build, test, packages. version comes from git describe.

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
PKGVER  := $(subst -,.,$(patsubst v%,%,$(VERSION)))
LDFLAGS := -s -w -X main.version=$(VERSION)
RPM     := dist/k2-$(PKGVER)-1.x86_64.rpm
DEB     := dist/k2_$(PKGVER)_amd64.deb

.PHONY: help build build-linux install test test-race vet fmt rpm deb dist clean demo

help:
	@echo "k2 $(VERSION)"
	@echo "  build  install  test  test-race  vet  fmt"
	@echo "  rpm    deb      dist  clean  demo"

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/k2 ./cmd/k2

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -trimpath -ldflags "$(LDFLAGS)" -o dist/k2-linux-amd64 ./cmd/k2

install:
	go install -trimpath -ldflags "$(LDFLAGS)" ./cmd/k2

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -s -w cmd internal

rpm: build-linux
	@command -v rpmbuild >/dev/null || { echo "need rpmbuild (dnf install rpm-build)"; exit 1; }
	rm -rf dist/rpmbuild
	rpmbuild -bb --target x86_64 \
		--define "_topdir $(CURDIR)/dist/rpmbuild" \
		--define "k2_version $(PKGVER)" \
		--define "k2_src $(CURDIR)" \
		packaging/rpm/k2.spec
	cp dist/rpmbuild/RPMS/x86_64/k2-$(PKGVER)-1.x86_64.rpm $(RPM)
	@echo "built $(RPM)"

deb: build-linux
	@command -v dpkg-deb >/dev/null || { echo "need dpkg-deb (apt install dpkg-dev)"; exit 1; }
	rm -rf dist/deb
	install -Dm0755 dist/k2-linux-amd64 dist/deb/usr/bin/k2
	install -Dm0644 config.example.yaml dist/deb/usr/share/k2/config.example.yaml
	install -Dm0644 README.md dist/deb/usr/share/doc/k2/README.md
	install -Dm0644 CHEATSHEET.md dist/deb/usr/share/doc/k2/CHEATSHEET.md
	install -Dm0644 LICENSE dist/deb/usr/share/doc/k2/LICENSE
	install -Dm0644 NOTICE dist/deb/usr/share/doc/k2/NOTICE
	install -Dm0644 packaging/deb/copyright dist/deb/usr/share/doc/k2/copyright
	mkdir -p dist/deb/DEBIAN
	sed 's/@VERSION@/$(PKGVER)/' packaging/deb/control > dist/deb/DEBIAN/control
	dpkg-deb --root-owner-group --build dist/deb $(DEB)
	@echo "built $(DEB)"

dist: rpm deb

demo:
	@command -v vhs >/dev/null || { echo "need vhs (https://github.com/charmbracelet/vhs)"; exit 1; }
	docs/demo/record.sh

clean:
	rm -rf bin dist
