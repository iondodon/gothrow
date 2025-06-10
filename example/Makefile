# Makefile for the gothrow example

# Go parameters
GOCMD=go
GORUN=$(GOCMD) run

# gothrow tool path
GOTHROW_TOOL=../gothrow

.PHONY: all gothrow run clean help

all: run

# Generate the error-handled code using gothrow
gothrow: ## Generate the error-handled code using gothrow
	@echo "Running gothrow to generate code..."
	rm -rf .out
	mkdir -p .out
	rsync -a --exclude .out ./ ./.out
	$(GOTHROW_TOOL) ./.out

# Run the generated code
run: gothrow ## Run the generated code
	@echo "Running generated code..."
	@cd .out && $(GORUN) .

# Clean up the generated code
clean: ## Clean up the generated code
	@echo "Cleaning up generated code..."
	rm -rf .out
