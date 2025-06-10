# gothrow

`gothrow` is a Go code generator that automatically adds error handling to your Go programs. It is a powerful, scope-aware tool that intelligently analyzes your code to safely handle ignored errors. For every function call where an error is ignored with an underscore (`_`), it injects the necessary error-checking boilerplate, while also ensuring that the generated code is always syntactically correct with respect to variable declarations.

## How it Works

The tool inspects all `.go` files in a given directory (and its subdirectories). It is designed to handle complex, real-world scenarios:

- **Finds Ignored Errors**: It identifies all assignments, whether they use `:=` or `=`, where an error type is discarded with `_`.

- **Scope-Aware Declaration**: It correctly determines if an `err` variable is already declared in the current scope.

  - If `err` is not declared, it uses `:=` to introduce it.
  - If `err` is already declared, it safely uses `=` to avoid "no new variables" compilation errors.

- **Handles Cascading Effects**: After introducing an `err` variable, it will automatically "demote" subsequent `err := ...` assignments in the same function to `err = ...` to prevent shadowing and other compiler errors.

- **Smart Returns**: In the error case, it returns the zero values for all return types of the enclosing function, followed by the `err` variable.

- **`main` function handling**: If the error is in the `main` function (which doesn't return anything), it injects a `log.Fatalf("error: %v", err)` call.

## Installation

You can install `gothrow` using `go install`:

```bash
go install github.com/iondodon/gothrow@latest
```

Alternatively, you can build it from the source:

```bash
make build
```

This will create a `gothrow` executable in the current directory.

## Usage

To run the tool, pass the directory you want to analyze as an argument. The tool will modify the files in-place. It is recommended to run it on a copy of your project or on a codebase that is under version control.

```bash
./gothrow [path/to/your/project]
```

### Example

Here is a more complex example that showcases the tool's capabilities.

**Before:**

```go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

func processRequest(r *http.Request) error {
	// This will be changed to `:=` because `err` is not yet defined.
	body, _ := io.ReadAll(r.Body)

	var data map[string]interface{}
	// This will be changed to `=` because `err` is now in scope.
	_ = json.Unmarshal(body, &data)

	// This is a valid `err :=` and should be demoted to `err =`.
	err := processData(data)
	if err != nil {
		log.Printf("Failed to process data: %v", err)
		return err
	}

	return nil
}

func processData(data map[string]interface{}) error {
	// Some processing logic...
	fmt.Println("Processing data:", data)
	return nil
}
```

**After running `gothrow`:**

```go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

func processRequest(r *http.Request) error {
	// Correctly uses `:=` to declare `err`.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}

	var data map[string]interface{}
	// Correctly uses `=` because `err` is already in scope.
	err = json.Unmarshal(body, &data)
	if err != nil {
		return err
	}

	// Correctly demoted the original `:=` to `=` to avoid a compiler error.
	err = processData(data)
	if err != nil {
		log.Printf("Failed to process data: %v", err)
		return err
	}

	return nil
}

func processData(data map[string]interface{}) error {
	// Some processing logic...
	fmt.Println("Processing data:", data)
	return nil
}
```
