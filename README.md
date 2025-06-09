# gothrow

`gothrow` is a Go code generator that automatically adds error handling to your Go programs. It analyzes your code and, for every function call where an error is ignored with an underscore (`_`), it injects the necessary error-checking boilerplate.

## How it Works

The tool inspects all `.go` files in a given directory (and its subdirectories). When it finds an assignment statement like this:

```go
data, _ := someFunction()
```

It rewrites it to:

```go
data, err := someFunction()
if err != nil {
    // ... return zero values and the error ...
}
```

The key features are:

- **Automatic Error Variable:** Replaces `_` with `err`.
- **Error Checking Block:** Inserts an `if err != nil` block.
- **Smart Returns:** In the error case, it returns the zero values for all return types of the enclosing function, followed by the `err` variable.
- **`main` function handling:** If the error is in the `main` function (which doesn't return anything), it injects a `log.Fatalf("error: %v", err)` call.
- **Safe:** It only adds error handling if the enclosing function is able to return an error, or if it is the `main` function.

## Installation

You can build `gothrow` from the source:

```bash
go build .
```

This will create a `gothrow` executable in the current directory.

## Usage

To run the tool, pass the directory you want to analyze as an argument. You can use `.` for the current directory.

```bash
./gothrow [path/to/your/project]
```

The tool will create an `out` directory inside your project folder containing the modified Go files, preserving the original structure. The original files are not modified.

### Example

The project includes an `example` directory to demonstrate its usage.

**Before:** `example/main.go`

```go
package main

import (
	"fmt"
	"os"
)

type A struct {
}

func main() {
	_, _ = os.ReadFile("test.txt")
	a, b, c, _ := anotherFuncWithIgnoredError()
	fmt.Println(a, b, c)
}

func anotherFunc() (int, string, error) {
	return 1, "hello", nil
}

func anotherFuncWithIgnoredError() (int, string, A, error) {
	i, s, _ := anotherFunc()
	return i, s, A{}, nil
}
```

**After running `./gothrow example`:** `example/out/main.go`

```go
package main

import (
	"fmt"
	"log"
	"os"
)

type A struct {
}

func main() {
	_, err := os.ReadFile("test.txt")
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	a, b, c, err := anotherFuncWithIgnoredError()
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	fmt.Println(a, b, c)
}

func anotherFunc() (int, string, error) {
	return 1, "hello", nil
}

func anotherFuncWithIgnoredError() (int, string, A, error) {
	i, s, err := anotherFunc()
	if err != nil {
		return 0, "", A{}, err
	}

	return i, s, A{}, nil
}
```
