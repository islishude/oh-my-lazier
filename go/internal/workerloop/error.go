package workerloop

import "errors"

// Fatal marks a worker loop error as non-retryable.
func Fatal(err error) error {
	if err == nil {
		return nil
	}
	return fatalError{err: err}
}

// IsFatal reports whether err was marked non-retryable.
func IsFatal(err error) bool {
	var target fatalError
	return errors.As(err, &target)
}

type fatalError struct {
	err error
}

func (e fatalError) Error() string {
	return e.err.Error()
}

func (e fatalError) Unwrap() error {
	return e.err
}
