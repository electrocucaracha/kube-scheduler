BINARY_NAME = scheduler
IMAGE = electrocucaracha/kube-scheduler
TAG = v0.1

# Build variables
BUILD_DIR ?= build

.PHONY: clean
clean: ## Clean the working area and the project
	@rm -rf ${BUILD_DIR}/${BINARY_NAME}

.PHONY: build
build: clean
	@go build -o ${BUILD_DIR}/${BINARY_NAME} ./cmd/main.go

.PHONY: test
test: build
	@go test -v ./...

.PHONY: docker-image
docker-image: clean
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ${BUILD_DIR}/${BINARY_NAME} ./cmd/main.go
	@docker build --rm --no-cache -t $(IMAGE):$(TAG) .