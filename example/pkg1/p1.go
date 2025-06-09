package pkg1

import (
	"errors"
	"fmt"
)

type A struct {
}

func P2() (int, string, error) {
	return 0, "", errors.New("erroroooorrrr")
}

func P1() (int, string, A, error) {
	i, s, _ := P2()
	fmt.Println(i, s)
	return i, s, A{}, nil
}