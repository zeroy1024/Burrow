//go:build !darwin && !linux

package fileclient

import "errors"

func renameLocalNoReplace(from, to string) error {
	return errors.ErrUnsupported
}
