package main

import (
	"fmt"
	"os"
)

func main() {
	content, _ := os.ReadFile("test.txt")
	fmt.Println(string(content))
}

func anotherFunc() (int, string, error) {
	return 1, "hello", nil
}

func anotherFuncWithIgnoredError() (int, string, error) {
	i, s, _ := anotherFunc()
	return i, s, nil
} 