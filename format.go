package serr

import (
	"fmt"
	"io"
	"reflect"
	"runtime"
	"strconv"
	"strings"
)

type StackFrame struct {
	Filename string
	Line     int
	FuncName string
}

func getStackFrame(pc uintptr) *StackFrame {
	if pc == 0 {
		return &StackFrame{}
	}

	frames := runtime.CallersFrames([]uintptr{pc})
	frame, _ := frames.Next()

	i := strings.LastIndex(frame.Function, "/")
	name := frame.Function[i+1:]

	return &StackFrame{
		Filename: frame.File,
		Line:     frame.Line,
		FuncName: name,
	}
}

type WrapLink struct {
	Msg     string
	MsgArgs []interface{}
	Fields  map[string]interface{}
	Frame   *StackFrame
}

type UnpackHierarchy struct {
	ErrExternal    error
	Stack          []*StackFrame
	Links          []*WrapLink
	SubHierarchies []*UnpackHierarchy
}

func (o *UnpackHierarchy) addStackFrame(stackFrame *StackFrame) {
	if stackFrame == nil {
		return
	}

	if len(o.Stack) > 0 && *o.Stack[len(o.Stack)-1] == *stackFrame {
		return
	}

	o.Stack = append(o.Stack, stackFrame)
}

func Unpack(err error) *UnpackHierarchy {
	return unpack(err, 0)
}

func unpack(err error, parentPC uintptr) *UnpackHierarchy {
	stackErr := Cause(err)
	if stackErr == nil {
		return &UnpackHierarchy{
			ErrExternal: err,
		}
	}

	stack := stackErr.(StackError).Callers()

	stackStartIndex := len(stack) - 1
	if parentPC != 0 {
		for stackStartIndex >= 0 {
			if stack[stackStartIndex] == parentPC {
				stackStartIndex--
				break
			}
			stackStartIndex--
		}
	}

	var stackFramError StackFrameError
	if e, ok := err.(StackFrameError); ok {
		if e.GetAdditionalInformation() != nil {
			stackFramError = e
		} else {
			stackFramError = nextStackFrameError(err)
		}
	}

	hierarchy := &UnpackHierarchy{}
	for stackIndex := stackStartIndex; stackIndex >= 0; stackIndex-- {
		for stackFramError != nil && stackIndex != len(stack)-1 && stack[stackIndex+1] == stackFramError.GetAdditionalInformation().callerCaller {
			additionalInformation := stackFramError.GetAdditionalInformation()
			stackFrame := getStackFrame(additionalInformation.caller)
			if len(additionalInformation.msg) != 0 || len(additionalInformation.msgArgs) != 0 || len(additionalInformation.fields) != 0 {
				hierarchy.Links = append(hierarchy.Links, &WrapLink{
					Msg:     additionalInformation.msg,
					MsgArgs: additionalInformation.msgArgs,
					Fields:  additionalInformation.fields,
					Frame:   stackFrame,
				})
			}
			hierarchy.addStackFrame(stackFrame)

			stackFramError = nextStackFrameError(stackFramError)
		}

		hierarchy.addStackFrame(getStackFrame(stack[stackIndex]))
	}

	if uerr, ok := stackErr.(interface{ Unwrap() error }); ok {
		hierarchy.SubHierarchies = append(hierarchy.SubHierarchies, unpack(uerr.Unwrap(), stack[1]))
	} else if uerr, ok := stackErr.(interface{ Unwrap() []error }); ok {
		for _, e := range uerr.Unwrap() {
			hierarchy.SubHierarchies = append(hierarchy.SubHierarchies, unpack(e, stack[1]))
		}
	}

	if len(hierarchy.SubHierarchies) == 1 {
		subHierarchies := hierarchy.SubHierarchies[0]
		hierarchy.Stack = append(hierarchy.Stack, subHierarchies.Stack...)
		hierarchy.Links = append(hierarchy.Links, subHierarchies.Links...)
		hierarchy.ErrExternal = subHierarchies.ErrExternal
		hierarchy.SubHierarchies = subHierarchies.SubHierarchies
	}

	return hierarchy
}

type FormatOptions struct {
	LocationFormatFunc func(frame *StackFrame) string
	WithTrace          bool
}

func DefaultLocationFormatFunc(frame *StackFrame) string {
	return frame.Filename + ":" + strconv.Itoa(frame.Line) + "(" + frame.FuncName + ")"
}

type JSONFormat struct {
	Options FormatOptions
}

func NewDefaultJSONFormat(options FormatOptions) JSONFormat {
	return JSONFormat{
		Options: options,
	}
}

func ToJSON(err error, withTrace bool) interface{} {
	return ToCustomJSON(err, NewDefaultJSONFormat(FormatOptions{
		LocationFormatFunc: DefaultLocationFormatFunc,
		WithTrace:          withTrace,
	}))
}

func ToCustomJSON(err error, format JSONFormat) interface{} {
	frames := unpack(err, 0)
	return toCustomJSON(frames, format)
}

func toCustomJSON(hierarchy *UnpackHierarchy, format JSONFormat) interface{} {
	root := map[string]interface{}{}
	if format.Options.WithTrace && len(hierarchy.Stack) > 0 {
		stackArr := []string{}
		for _, stack := range hierarchy.Stack {
			src := format.Options.LocationFormatFunc(stack)
			stackArr = append(stackArr, src)
		}
		root["stack"] = stackArr
	}
	if len(hierarchy.Links) > 0 {
		wrapArr := []interface{}{}
		for _, link := range hierarchy.Links {
			wrapMap := map[string]interface{}{
				"msg": fmt.Sprintf(link.Msg, link.MsgArgs...),
			}
			if len(link.Fields) != 0 {
				wrapMap["fields"] = link.Fields
			}
			if format.Options.WithTrace {
				wrapMap["src"] = format.Options.LocationFormatFunc(link.Frame)
			}
			wrapArr = append(wrapArr, wrapMap)
		}
		root["wrap"] = wrapArr
	}
	if hierarchy.ErrExternal != nil {
		root["external"] = fmt.Sprint(hierarchy.ErrExternal)
		if format.Options.WithTrace {
			root["type"] = reflect.TypeOf(hierarchy.ErrExternal).String()
		}
	}
	if len(hierarchy.SubHierarchies) > 0 {
		subArr := []interface{}{}
		for _, subHierarchy := range hierarchy.SubHierarchies {
			subArr = append(subArr, toCustomJSON(subHierarchy, format))
		}
		root["join"] = subArr
	}

	return root
}

func DefaultFieldFormat(fields map[string]interface{}) string {
	return fmt.Sprint(fields)
}

type StringFormat struct {
	Options         FormatOptions
	MsgStackSep     string // Separator between error messages and stack frame data.
	PreStackSep     string // Separator at the beginning of each stack frame.
	StackElemSep    string // Separator between elements of each stack frame.
	ErrorSep        string // Separator between each error in the chain.
	PreFieldSep     string // Separator at the beginning of each field.
	FieldFormatFunc func(fields map[string]interface{}) string
}

func NewDefaultStringFormat(options FormatOptions) StringFormat {
	format := StringFormat{
		Options:         options,
		FieldFormatFunc: DefaultFieldFormat,
	}
	if options.WithTrace {
		format.MsgStackSep = "\n"
		format.PreStackSep = "\t"
		format.StackElemSep = "\n"
		format.ErrorSep = "\n"
		format.PreFieldSep = " "
	} else {
		format.ErrorSep = ": "
		format.PreFieldSep = " "
	}
	return format
}

func ToString(err error, withTrace bool) string {
	return ToCustomString(err, NewDefaultStringFormat(FormatOptions{
		LocationFormatFunc: DefaultLocationFormatFunc,
		WithTrace:          withTrace,
	}))
}

func ToCustomString(err error, format StringFormat) string {
	frames := unpack(err, 0)
	return toCustomString(frames, format, 1)
}

func toCustomString(hierarchy *UnpackHierarchy, format StringFormat, level int) string {
	str := ""
	stackIndex := 0
	for _, link := range hierarchy.Links {
		if format.Options.WithTrace {
			if stackIndex == 0 || *hierarchy.Stack[stackIndex-1] != *link.Frame {
				for stackIndex < len(hierarchy.Stack)-1 && *hierarchy.Stack[stackIndex] != *link.Frame {
					str += strings.Repeat(format.PreStackSep, level) + format.Options.LocationFormatFunc(hierarchy.Stack[stackIndex]) + format.StackElemSep
					stackIndex++
				}
				str += strings.Repeat(format.PreStackSep, level) + format.Options.LocationFormatFunc(link.Frame) + format.MsgStackSep
				stackIndex++
			}
		}
		str += fmt.Sprintf(link.Msg, link.MsgArgs...)
		if len(link.Fields) != 0 {
			str += format.PreFieldSep + format.FieldFormatFunc(link.Fields)
		}
		str += format.ErrorSep
	}
	if format.Options.WithTrace {
		for stackIndex < len(hierarchy.Stack) {
			str += strings.Repeat(format.PreStackSep, level) + format.Options.LocationFormatFunc(hierarchy.Stack[stackIndex]) + format.StackElemSep
			stackIndex++
		}
	}
	if hierarchy.ErrExternal != nil {
		str += fmt.Sprint(hierarchy.ErrExternal) + format.ErrorSep
	}
	for i, subHierarchy := range hierarchy.SubHierarchies {
		if format.Options.WithTrace {
			str += strings.Repeat(format.PreStackSep, level) + "#" + strconv.Itoa(i) + format.StackElemSep
		}
		str += toCustomString(subHierarchy, format, level+1)
	}

	return str
}

func printError(err error, s fmt.State, verb rune) {
	var withTrace bool
	switch verb {
	case 'v':
		if s.Flag('+') {
			withTrace = true
		}
	}
	str := ToString(err, withTrace)
	_, _ = io.WriteString(s, str)
}
