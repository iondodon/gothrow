# Makefile for gothrow

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOINSTALL=$(GOCMD) install
BINARY_NAME=gothrow

.PHONY: all build clean install help

all: build

# Build the gothrow binary
build: ## Build the gothrow binary
	@echo "Building $(BINARY_NAME)..."
	$(GOBUILD) -o $(BINARY_NAME) .

install: ## Install the gothrow binary
	@echo "Installing $(BINARY_NAME)..."
	$(GOINSTALL) .

# Clean up build artifacts
clean: ## Clean up build artifacts
	@echo "Cleaning up..."
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

help: ## Show this help message
	@echo "Usage: make [target]"
	@echo
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "}; /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST) 