package main

import (
	"errors"
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
	return 0, "", errors.New("erroroooorrrr")
}

func anotherFuncWithIgnoredError() (int, string, A, error) {
	i, s, _ := anotherFunc()
	return i, s, A{}, nil
} 