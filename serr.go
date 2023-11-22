package serr

import (
	"fmt"
	"runtime"
)

type Fields map[string]interface{}

func getCallers(skip int) []uintptr {
	const depth = 64
	var pcs [depth]uintptr
	n := runtime.Callers(skip, pcs[:])
	return pcs[0:n]
}

type StackError interface {
	error
	Callers() []uintptr
}

type AdditionalInformation struct {
	caller       uintptr
	callerCaller uintptr
	fields       map[string]interface{}
	msg          string
	msgArgs      []interface{}
}

func getAdditionalInformation(callers []uintptr) *AdditionalInformation {
	info := &AdditionalInformation{}

	if len(callers) > 0 {
		info.caller = callers[0]
	}

	if len(callers) > 1 {
		info.callerCaller = callers[1]
	}

	return info
}

type StackFrameError interface {
	error
	GetAdditionalInformation() *AdditionalInformation
}

func nextStackFrameError(err error) StackFrameError {
	for err != nil {
		uerr, ok := err.(interface{ Unwrap() error })
		if !ok {
			return nil
		}

		stackFrameError, ok := uerr.Unwrap().(StackFrameError)
		if !ok {
			return nil
		}

		if stackFrameError.GetAdditionalInformation() != nil {
			return stackFrameError
		}
	}

	return nil
}

type rootError struct {
	callers               []uintptr
	additionalInformation *AdditionalInformation
	err                   error
}

func New(fields map[string]interface{}, msg string) error {
	callers := getCallers(3) // skip New, getCallers, runtime.Callers
	additionalInformation := getAdditionalInformation(callers)
	additionalInformation.fields = fields
	additionalInformation.msg = msg

	return &rootError{
		callers:               callers,
		additionalInformation: additionalInformation,
	}
}

func (e *rootError) Error() string {
	return fmt.Sprint(e)
}

func (e *rootError) Format(s fmt.State, verb rune) {
	printError(e, s, verb)
}

func (e *rootError) Unwrap() error {
	return e.err
}

func (e *rootError) Callers() []uintptr {
	return e.callers
}

func (e *rootError) GetAdditionalInformation() *AdditionalInformation {
	return e.additionalInformation
}

type wrapError struct {
	additionalInformation *AdditionalInformation
	err                   error
}

func Wrap(err error) error {
	switch err := err.(type) {
	case StackError:
		return err
	case StackFrameError:
		return err
	default:
		callers := getCallers(3) // skip Wrap, getCallers, runtime.Callers
		return &rootError{
			callers: callers,
			err:     err,
		}
	}
}

func Wrapf(err error, fields map[string]interface{}, msg string, msgArgs ...interface{}) error {
	return WrapDepth(err, 1, fields, msg, msgArgs...)
}

func WrapDepth(err error, skip int, fields map[string]interface{}, msg string, msgArgs ...interface{}) error {
	callers := getCallers(3 + skip) // skip WrapDepth, getCallers, runtime.Callers
	additionalInformation := getAdditionalInformation(callers)
	additionalInformation.fields = fields
	additionalInformation.msg = msg
	additionalInformation.msgArgs = msgArgs

	switch err := err.(type) {
	case *rootError:
	case *wrapError:
	case *joinError:
	default:
		return &rootError{
			callers:               callers,
			additionalInformation: additionalInformation,
			err:                   err,
		}
	}

	return &wrapError{
		additionalInformation: additionalInformation,
		err:                   err,
	}
}

func (e *wrapError) Error() string {
	return fmt.Sprint(e)
}

func (e *wrapError) Format(s fmt.State, verb rune) {
	printError(e, s, verb)
}

func (e *wrapError) Unwrap() error {
	return e.err
}

func (e *wrapError) GetAdditionalInformation() *AdditionalInformation {
	return e.additionalInformation
}

type joinError struct {
	callers []uintptr
	errs    []error
}

func Join(errs ...error) error {
	return JoinDepth(1, errs...)
}

func JoinDepth(skip int, errs ...error) error {
	n := 0
	for _, err := range errs {
		if err != nil {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	if n == 1 {
		for _, err := range errs {
			if err != nil {
				return &rootError{
					callers: getCallers(skip + 3), // skip Join, getCallers, runtime.Callers
					err:     err,
				}
			}
		}
	}

	e := &joinError{
		callers: getCallers(skip + 3), // skip Join, getCallers, runtime.Callers
		errs:    make([]error, 0, n),
	}
	for _, err := range errs {
		if err != nil {
			e.errs = append(e.errs, err)
		}
	}
	return e
}

func (e *joinError) Error() string {
	return fmt.Sprint(e)
}

func (e *joinError) Format(s fmt.State, verb rune) {
	printError(e, s, verb)
}

func (e *joinError) Unwrap() []error {
	return e.errs
}

func (e *joinError) Callers() []uintptr {
	return e.callers
}

// get the first StackError, return nil if not found
func Cause(err error) error {
	for err != nil {
		if _, ok := err.(StackError); ok {
			return err
		}

		if uerr, ok := err.(interface{ Unwrap() error }); !ok {
			return nil
		} else {
			err = uerr.Unwrap()
		}
	}

	return nil
}
