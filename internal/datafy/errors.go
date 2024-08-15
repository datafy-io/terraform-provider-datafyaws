package datafy

import (
	"errors"
)

var (
	NotFoundError = errors.New("couldn't find resource")
)

func NotFound(err error) bool {
	return errors.Is(err, NotFoundError)
}
