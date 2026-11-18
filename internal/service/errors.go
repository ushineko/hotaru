package service

import (
	"errors"

	"github.com/ushineko/hotaru/internal/devices"
)

func asUnsupported(err error, target **devices.Unsupported) bool {
	return errors.As(err, target)
}
