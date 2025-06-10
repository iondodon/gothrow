# Makefile for gothrow

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
BINARY_NAME=gothrow
EXAMPLE_DIR=example

.PHONY: all build run clean

all: build

# Build the gothrow binary
build:
	@echo "Building $(BINARY_NAME)..."
	$(GOBUILD) -o $(BINARY_NAME) .

# Run gothrow on the example directory
run: build
	@echo "Preparing out directory..."
	rm -rf $(EXAMPLE_DIR)/out
	mkdir -p $(EXAMPLE_DIR)/out
	cp -r $(EXAMPLE_DIR)/* $(EXAMPLE_DIR)/out/
	@echo "Running $(BINARY_NAME) on $(EXAMPLE_DIR)/out..."
	./$(BINARY_NAME) $(EXAMPLE_DIR)/out

# Clean up build artifacts and generated code
clean:
	@echo "Cleaning up..."
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -rf $(EXAMPLE_DIR)/out 