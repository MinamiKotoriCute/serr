package serr

import (
	"fmt"
	"runtime"
)

func NewCallers(skip int) []uintptr {
	const depth = 64
	var pcs [depth]uintptr
	n := runtime.Callers(skip, pcs[:])
	return pcs[0:n]
}

type FullStackError interface {
	error
	Callers() []uintptr
}

type SimpleFullStackError struct {
	callers []uintptr
}

var _ FullStackError = (*SimpleFullStackError)(nil)

func (o *SimpleFullStackError) Callers() []uintptr {
	return o.callers
}

func (o *SimpleFullStackError) Error() string {
	return "simple full stack error"
}

func NewSimpleFullStackError(skip int) SimpleFullStackError {
	return SimpleFullStackError{
		callers: NewCallers(skip + 3), // skip NewSimpleFullStackError, NewCallers, runtime.Callers
	}
}

type ExtraStackData struct {
	caller       uintptr
	callerCaller uintptr
	fields       map[string]interface{}
	msg          string
	msgArgs      []interface{}
}

func NewExtraStackData(skip int) *ExtraStackData {
	var pcs [2]uintptr
	n := runtime.Callers(skip, pcs[:])

	extraStackData := &ExtraStackData{}

	if n > 0 {
		extraStackData.caller = pcs[0]
	}

	if n > 1 {
		extraStackData.callerCaller = pcs[1]
	}

	return extraStackData
}

func NewExtraStackDataFromCallers(callers []uintptr) *ExtraStackData {
	extraStackData := &ExtraStackData{}

	if len(callers) > 0 {
		extraStackData.caller = callers[0]
	}

	if len(callers) > 1 {
		extraStackData.callerCaller = callers[1]
	}

	return extraStackData
}

type ExtraStackError interface {
	error
	GetExtraStackData() *ExtraStackData
}

type SimpleExtraStackError struct {
	extraStackData *ExtraStackData
}

var _ ExtraStackError = (*SimpleExtraStackError)(nil)

func (o *SimpleExtraStackError) GetExtraStackData() *ExtraStackData {
	return o.extraStackData
}

func (o *SimpleExtraStackError) Error() string {
	if o.extraStackData == nil {
		return "simple extra stack error"
	} else {
		return fmt.Sprintf(o.extraStackData.msg, o.extraStackData.msgArgs...)
	}
}

func NewSimpleExtraStackError(skip int) SimpleExtraStackError {
	return SimpleExtraStackError{
		extraStackData: NewExtraStackData(skip + 1), // skip NewSimpleExtraStackError
	}
}

type rootError struct {
	SimpleFullStackError
	SimpleExtraStackError
	err error
}

var _ FullStackError = (*rootError)(nil)
var _ ExtraStackError = (*rootError)(nil)

func New(msg string) error {
	return ErrorDepthsWrapError(1, nil, nil, msg)
}

func Errorf(msg string, args ...interface{}) error {
	return ErrorDepthsWrapError(1, nil, nil, msg, args...)
}

func Errors(fields map[string]interface{}, msg string, args ...interface{}) error {
	return ErrorDepthsWrapError(1, nil, fields, msg, args...)
}

func ErrorDepth(skip int, msg string) error {
	return ErrorDepthsWrapError(skip+1, nil, nil, msg)
}

func ErrorDepthf(skip int, msg string, args ...interface{}) error {
	return ErrorDepthsWrapError(skip+1, nil, nil, msg, args...)
}

func ErrorDepths(skip int, fields map[string]interface{}, msg string, args ...interface{}) error {
	return ErrorDepthsWrapError(skip+1, nil, fields, msg, args...)
}

func ErrorDepthsWrapError(skip int, err error, fields map[string]interface{}, msg string, args ...interface{}) error {
	simpleFullStackErr := NewSimpleFullStackError(skip + 1) // skip ErrorDepthsWrapError
	extraStackData := NewExtraStackDataFromCallers(simpleFullStackErr.Callers())
	extraStackData.fields = fields
	extraStackData.msg = msg
	extraStackData.msgArgs = args

	return &rootError{
		SimpleFullStackError: simpleFullStackErr,
		SimpleExtraStackError: SimpleExtraStackError{
			extraStackData: extraStackData,
		},
		err: err,
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

type wrapError struct {
	SimpleExtraStackError
	err error
}

var _ ExtraStackError = (*wrapError)(nil)

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
	return WrapDepths(skip+1, err, nil, "")
}

func WrapDepthf(skip int, err error, msg string, msgArgs ...interface{}) error {
	return WrapDepths(skip+1, err, nil, msg, msgArgs...)
}

func WrapDepths(skip int, err error, fields map[string]interface{}, msg string, msgArgs ...interface{}) error {
	if Cause(err) != nil {
		extraStackData := NewExtraStackData(skip + 3) // skip WrapDepths, NewExtraStackData, runtime.Callers
		extraStackData.fields = fields
		extraStackData.msg = msg
		extraStackData.msgArgs = msgArgs

		return &wrapError{
			SimpleExtraStackError: SimpleExtraStackError{
				extraStackData: extraStackData,
			},
			err: err,
		}
	}

	return ErrorDepthsWrapError(skip+1, err, nil, "")
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

type joinError struct {
	SimpleFullStackError
	errs []error
}

var _ FullStackError = (*joinError)(nil)

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
				return ErrorDepthsWrapError(skip+1, err, nil, "")
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
		SimpleFullStackError: SimpleFullStackError{
			callers: NewCallers(skip + 3), // skip JoinDepth, NewCallers, runtime.Callers
		},
		errs: make([]error, 0, n),
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

// get the first FullStackError, return nil if not found
func Cause(err error) error {
	for err != nil {
		if fullStackErr, ok := err.(FullStackError); ok && len(fullStackErr.Callers()) > 0 {
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

func NextExtraStackError(err error) ExtraStackError {
	for err != nil {
		uerr, ok := err.(interface{ Unwrap() error })
		if !ok {
			return nil
		}

		err = uerr.Unwrap()

		stackFrameError, ok := err.(ExtraStackError)
		if !ok {
			return nil
		}

		if stackFrameError.GetExtraStackData() != nil {
			return stackFrameError
		}
	}

	return nil
}
