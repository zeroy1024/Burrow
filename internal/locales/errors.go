package locales

import "fmt"

// messageError keeps app-owned errors in the current language while
// retaining the original error chain for errors.Is and errors.As.
type messageError struct {
	message func() string
	cause   error
}

func (e *messageError) Error() string { return e.message() }
func (e *messageError) Unwrap() error { return e.cause }

func NewError(message func() string) error {
	return &messageError{message: message}
}

func Errorf(format func() string, args ...any) error {
	return &messageError{
		message: func() string { return fmt.Errorf(format(), args...).Error() },
		cause:   fmt.Errorf(format(), args...),
	}
}
