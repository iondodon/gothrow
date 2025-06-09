# Makefile for the gothrow example

# Go parameters
GOCMD=go
GORUN=$(GOCMD) run

# gothrow tool path
GOTHROW_TOOL=../gothrow

.PHONY: all generate run run-original clean

all: run

# Generate the error-handled code using gothrow
generate:
	@echo "Running gothrow to generate code..."
	$(GOTHROW_TOOL) .

# Run the generated code
run: generate
	@echo "Running generated code..."
	@cd out && $(GORUN) .

# Run the original example code
run-original:
	@echo "Running original code..."
	$(GORUN) .

# Clean up the generated code
clean:
	@echo "Cleaning up generated code..."
	rm -rf out 