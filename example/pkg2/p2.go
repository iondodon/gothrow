package pkg2

import "errors"

func P3() error {
	return errors.New("an error from pkg2")
} 