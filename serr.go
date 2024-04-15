package serr

import (
	"fmt"
	"runtime"
)

type Fields map[string]interface{}

func NewCallers(skip int) []uintptr {
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

func NewAdditionalInformationFromCallers(callers []uintptr) *AdditionalInformation {
	info := &AdditionalInformation{}

	if len(callers) > 0 {
		info.caller = callers[0]
	}

	if len(callers) > 1 {
		info.callerCaller = callers[1]
	}

	return info
}

func NewAdditionalInformation(skip int) *AdditionalInformation {
	var pcs [2]uintptr
	n := runtime.Callers(skip, pcs[:])

	info := &AdditionalInformation{}

	if n > 0 {
		info.caller = pcs[0]
	}

	if n > 1 {
		info.callerCaller = pcs[1]
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

		err = uerr.Unwrap()

		stackFrameError, ok := err.(StackFrameError)
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

var _ StackError = (*rootError)(nil)
var _ StackFrameError = (*rootError)(nil)

func New(msg string) error {
	return ErrorDepths(1, nil, msg)
}

func Errorf(msg string, args ...interface{}) error {
	return ErrorDepths(1, nil, msg, args...)
}

func Errors(fields map[string]interface{}, msg string, args ...interface{}) error {
	return ErrorDepths(1, fields, msg, args...)
}

func ErrorDepth(skip int, msg string) error {
	return ErrorDepths(skip+1, nil, msg)
}

func ErrorDepthf(skip int, fields map[string]interface{}, msg string, args ...interface{}) error {
	return ErrorDepths(skip+1, fields, msg, args...)
}

func ErrorDepths(skip int, fields map[string]interface{}, msg string, args ...interface{}) error {
	callers := NewCallers(skip + 3) // skip ErrorDepths, NewCallers, runtime.Callers
	additionalInformation := NewAdditionalInformationFromCallers(callers)
	additionalInformation.fields = fields
	additionalInformation.msg = msg
	additionalInformation.msgArgs = args

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

var _ StackFrameError = (*wrapError)(nil)

func Wrap(err error) error {
	return WrapDepth(1, err)
}

func Wrapf(err error, msg string, msgArgs ...interface{}) error {
	return WrapDepths(1, err, nil, msg, msgArgs...)
}

func Wraps(err error, fields map[string]interface{}, msg string, msgArgs ...interface{}) error {
	return WrapDepths(1, err, fields, msg, msgArgs...)
}

func WrapDepth(skip int, err error) error {
	if Cause(err) != nil {
		return &wrapError{
			additionalInformation: NewAdditionalInformation(skip + 3), // skip WrapDepth, newAdditionalInformation, runtime.Callers
			err:                   err,
		}
	}

	return &rootError{
		callers: NewCallers(skip + 3), // skip WrapDepth, NewCallers, runtime.Callers
		err:     err,
	}
}

func WrapDepthf(skip int, err error, msg string, msgArgs ...interface{}) error {
	return WrapDepths(skip+1, err, nil, msg, msgArgs...)
}

func WrapDepths(skip int, err error, fields map[string]interface{}, msg string, msgArgs ...interface{}) error {
	if Cause(err) != nil {
		additionalInformation := NewAdditionalInformation(skip + 3) // skip WrapDepths, newAdditionalInformation, runtime.Callers
		additionalInformation.fields = fields
		additionalInformation.msg = msg
		additionalInformation.msgArgs = msgArgs

		return &wrapError{
			additionalInformation: additionalInformation,
			err:                   err,
		}
	}

	callers := NewCallers(skip + 3) // skip WrapDepths, NewCallers, runtime.Callers
	additionalInformation := NewAdditionalInformationFromCallers(callers)
	additionalInformation.fields = fields
	additionalInformation.msg = msg
	additionalInformation.msgArgs = msgArgs

	return &rootError{
		callers:               callers,
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

var _ StackError = (*joinError)(nil)

func Join(errs ...error) error {
	return JoinDepth(1, errs...)
}

// join errors into first error
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
					callers: NewCallers(skip + 3), // skip Join, NewCallers, runtime.Callers
					err:     err,
				}
			}
		}
	}

	if jerr, ok := errs[0].(*joinError); ok {
		for _, err := range errs[1:] {
			if err != nil {
				jerr.errs = append(jerr.errs, err)
			}
		}
		return jerr
	}

	e := &joinError{
		callers: NewCallers(skip + 3), // skip Join, NewCallers, runtime.Callers
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
		if stackErr, ok := err.(StackError); ok && len(stackErr.Callers()) > 0 {
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
