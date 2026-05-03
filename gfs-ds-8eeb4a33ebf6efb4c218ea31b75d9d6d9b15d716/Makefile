PROTO_DIR = api/proto
CHUNK_MASTER = chunk_master
BIN_DIR = bin
OUT_DIR = ./$(BIN_DIR)

PROTO_FILE_CHUNK_MASTER = $(PROTO_DIR)/chunk_master/chunk_master.proto
PROTO_FILE_CHUNK_OPERATIONS = $(PROTO_DIR)/chunk_operations/chunk_operations.proto
PROTO_FILE_CLIENT_MASTER = $(PROTO_DIR)/client_master/client_master.proto
PROTO_FILE_COMMON = $(PROTO_DIR)/common/common.proto
PROTO_FILE_CHUNK = $(PROTO_DIR)/chunk/chunk.proto

PROTO_OUT_DIR = .

GO_FLAGS = --go_out=$(PROTO_OUT_DIR) --go_opt=paths=source_relative \
           --go-grpc_out=$(PROTO_OUT_DIR) --go-grpc_opt=paths=source_relative

GO_BUILD_FLAGS = CGO_ENABLED=0 GOOS=linux GOARCH=amd64

.PHONY: proto clean build build-all build-client build-master build-chunkserver help

proto:
	protoc $(GO_FLAGS) $(PROTO_FILE_CHUNK_MASTER) $(PROTO_FILE_CHUNK_OPERATIONS) $(PROTO_FILE_CLIENT_MASTER) $(PROTO_FILE_COMMON) $(PROTO_FILE_CHUNK)

clean:
	find $(PROTO_DIR) -type f -name '*.pb.go' -exec rm -rf {} +
	rm -rf $(BIN_DIR)

build-client: proto
	@mkdir -p $(BIN_DIR)
	@echo "Building GFS client..."
	$(GO_BUILD_FLAGS) go build -o $(OUT_DIR)/gfs-client ./cmd/client

build-master: proto
	@mkdir -p $(BIN_DIR)
	@echo "Building GFS master..."
	$(GO_BUILD_FLAGS) go build -o $(OUT_DIR)/gfs-master ./cmd/master

build-chunkserver: proto
	@mkdir -p $(BIN_DIR)
	@echo "Building GFS chunkserver..."
	$(GO_BUILD_FLAGS) go build -o $(OUT_DIR)/gfs-chunkserver ./cmd/chunkserver

build-all: proto build-client build-master build-chunkserver
	@echo "✓ All binaries built successfully"
	@ls -lh $(BIN_DIR)/

build: build-all

help:
	@echo "GFS Build Targets:"
	@echo "  make proto              - Generate protobuf code"
	@echo "  make build-client       - Build only client binary"
	@echo "  make build-master       - Build only master binary"
	@echo "  make build-chunkserver  - Build only chunkserver binary"
	@echo "  make build-all          - Build all binaries (default)"
	@echo "  make build              - Alias for build-all"
	@echo "  make clean              - Remove generated code and binaries"
	@echo "  make help               - Show this help message"

