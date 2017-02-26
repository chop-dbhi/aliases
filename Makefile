PROG_NAME := "aliases"

IMAGE_NAME := "dbhi/aliases"

GIT_SHA := $(shell git log -1 --pretty=format:"%h" .)
GIT_TAG := $(shell git describe --tags --exact-match . 2>/dev/null)
GIT_BRANCH := $(shell git symbolic-ref -q --short HEAD)
GIT_VERSION := $(shell git log -1 --pretty=format:"%h (%ci)" .)

setup: glide tls compiledaemon

glide:
	@if command -v glide &> /dev/null; then \
		echo >&2 'Installing library dependences'; \
		glide install; \
	else \
		echo >&2 'Glide required: https://glide.sh'; \
		exit 1; \
	fi

tls:
	@if [ ! -a cert.pem ]; then \
		echo >&2 'Creating self-signed TLS certs.'; \
		go run $(shell go env GOROOT)/src/crypto/tls/generate_cert.go --host localhost; \
	fi

compiledaemon:
	@if command -v CompileDaemon &> /dev/null; then \
		echo >&2 'Getting CompileDaemon for auto-reload.'; \
		go get github.com/githubnemo/CompileDaemon; \
	fi

watch:
	CompileDaemon \
		-build="make build" \
		-command="$(PROG_NAME)" \
		-graceful-kill=true \
		-exclude-dir=.git \
		-exclude-dir=vendor \
		-color=true

build:
	go build -ldflags "-X \"main.buildVersion=$(GIT_VERSION)\"" \
		-o $(GOPATH)/bin/$(PROG_NAME) .

dist-build:
	mkdir -p dist

	gox -output="./dist/{{.OS}}-{{.Arch}}/$(PROG_NAME)" \
		-ldflags "-X \"main.buildVersion=$(GIT_VERSION)\"" \
		-os "windows linux darwin" \
		-arch "amd64" . > /dev/null

dist-zip:
	cd dist && zip $(PROG_NAME)-darwin-amd64.zip darwin-amd64/*
	cd dist && zip $(PROG_NAME)-linux-amd64.zip linux-amd64/*
	cd dist && zip $(PROG_NAME)-windows-amd64.zip windows-amd64/*

dist: dist-build dist-zip

docker:
	docker build -t ${IMAGE_NAME}:${GIT_SHA} .
	docker tag ${IMAGE_NAME}:${GIT_SHA} ${IMAGE_NAME}:${GIT_BRANCH}
	if [ -n "${GIT_TAG}" ] ; then \
		docker tag ${IMAGE_NAME}:${GIT_SHA} ${IMAGE_NAME}:${GIT_TAG} ; \
	fi;
	if [ "${GIT_BRANCH}" == "master" ]; then \
		docker tag ${IMAGE_NAME}:${GIT_SHA} ${IMAGE_NAME}:latest ; \
	fi;

docker-push:
	docker push ${IMAGE_NAME}:${GIT_SHA}
	docker push ${IMAGE_NAME}:${GIT_BRANCH}
	if [ -n "${GIT_TAG}" ]; then \
		docker push ${IMAGE_NAME}:${GIT_TAG} ; \
	fi;
	if [ "${GIT_BRANCH}" == "master" ]; then \
		docker push ${IMAGE_NAME}:latest ; \
	fi;

.PHONY: build dist-build dist
