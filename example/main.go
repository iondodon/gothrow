package main

import (
	"fmt"
	"os"

	"gothrow/example/pkg1"
)

func main() {
	_, _ = os.ReadFile("test.txt")
	a, b, c, _ := anotherFuncWithIgnoredError()
	fmt.Println(a, b, c)
}

func anotherFuncWithIgnoredError() (int, string, pkg1.A, error) {
	i, s, a, _ := pkg1.P1()
	return i, s, a, nil
} 