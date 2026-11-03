//go:build !darwin && !linux && !windows

package fileclient

import "errors"

func renameLocalNoReplace(from, to string) error {
	return errors.ErrUnsupported
}
