package serr

import (
	"fmt"
	"io"
	"reflect"
	"runtime"
	"strconv"
	"strings"
)

type Location struct {
	Filename string
	Line     int
	FuncName string
}

func getLocation(pc uintptr) *Location {
	if pc == 0 {
		return &Location{}
	}

	frames := runtime.CallersFrames([]uintptr{pc})
	frame, _ := frames.Next()

	i := strings.LastIndex(frame.Function, "/")
	name := frame.Function[i+1:]

	return &Location{
		Filename: frame.File,
		Line:     frame.Line,
		FuncName: name,
	}
}

type WrapLink struct {
	Msg            string
	MsgArgs        []interface{}
	Fields         map[string]interface{}
	CallerLocation *Location
}

type UnpackHierarchy struct {
	ErrExternal     error
	CallerLocations []*Location
	Links           []*WrapLink
	SubHierarchies  []*UnpackHierarchy
}

func (o *UnpackHierarchy) addCallerLocation(callerLocation *Location) {
	if callerLocation == nil {
		return
	}

	if len(o.CallerLocations) > 0 && *o.CallerLocations[len(o.CallerLocations)-1] == *callerLocation {
		return
	}

	o.CallerLocations = append(o.CallerLocations, callerLocation)
}

func Unpack(err error) *UnpackHierarchy {
	return unpack(err, 0)
}

func unpack(err error, parentPC uintptr) *UnpackHierarchy {
	fullStackErr := Cause(err)
	if fullStackErr == nil {
		return &UnpackHierarchy{
			ErrExternal: err,
		}
	}

	callers := fullStackErr.(FullStackError).Callers()

	callerStartIndex := len(callers) - 1
	if parentPC != 0 {
		for callerStartIndex >= 0 {
			if callers[callerStartIndex] == parentPC {
				callerStartIndex--
				break
			}
			callerStartIndex--
		}
	}

	var extraStackErr ExtraStackError
	if e, ok := err.(ExtraStackError); ok {
		if e.GetExtraStackData() != nil {
			extraStackErr = e
		} else {
			extraStackErr = NextExtraStackError(err)
		}
	}

	hierarchy := &UnpackHierarchy{}
	for callerIndex := callerStartIndex; callerIndex >= 0; callerIndex-- {
		for extraStackErr != nil && callerIndex != len(callers)-1 && callers[callerIndex+1] == extraStackErr.GetExtraStackData().callerCaller {
			extraStackData := extraStackErr.GetExtraStackData()
			callerLocation := getLocation(extraStackData.caller)
			if len(extraStackData.msg) != 0 || len(extraStackData.msgArgs) != 0 || len(extraStackData.fields) != 0 {
				hierarchy.Links = append(hierarchy.Links, &WrapLink{
					Msg:            extraStackData.msg,
					MsgArgs:        extraStackData.msgArgs,
					Fields:         extraStackData.fields,
					CallerLocation: callerLocation,
				})
			}
			hierarchy.addCallerLocation(callerLocation)

			extraStackErr = NextExtraStackError(extraStackErr)
		}

		hierarchy.addCallerLocation(getLocation(callers[callerIndex]))
	}

	if uerr, ok := fullStackErr.(interface{ Unwrap() error }); ok {
		hierarchy.SubHierarchies = append(hierarchy.SubHierarchies, unpack(uerr.Unwrap(), callers[1]))
	} else if uerr, ok := fullStackErr.(interface{ Unwrap() []error }); ok {
		for _, e := range uerr.Unwrap() {
			hierarchy.SubHierarchies = append(hierarchy.SubHierarchies, unpack(e, callers[1]))
		}
	}

	if len(hierarchy.SubHierarchies) == 1 {
		subHierarchies := hierarchy.SubHierarchies[0]
		hierarchy.CallerLocations = append(hierarchy.CallerLocations, subHierarchies.CallerLocations...)
		hierarchy.Links = append(hierarchy.Links, subHierarchies.Links...)
		hierarchy.ErrExternal = subHierarchies.ErrExternal
		hierarchy.SubHierarchies = subHierarchies.SubHierarchies
	}

	return hierarchy
}

type FormatOptions struct {
	LocationFormatFunc func(frame *Location) string
	WithTrace          bool
}

func DefaultLocationFormatFunc(frame *Location) string {
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
	if format.Options.WithTrace && len(hierarchy.CallerLocations) > 0 {
		stackArr := []string{}
		for _, stack := range hierarchy.CallerLocations {
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
				wrapMap["src"] = format.Options.LocationFormatFunc(link.CallerLocation)
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
	} else {
		format.ErrorSep = ": "
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
			if stackIndex == 0 || *hierarchy.CallerLocations[stackIndex-1] != *link.CallerLocation {
				for stackIndex < len(hierarchy.CallerLocations)-1 && *hierarchy.CallerLocations[stackIndex] != *link.CallerLocation {
					str += strings.Repeat(format.PreStackSep, level) + format.Options.LocationFormatFunc(hierarchy.CallerLocations[stackIndex]) + format.StackElemSep
					stackIndex++
				}
				str += strings.Repeat(format.PreStackSep, level) + format.Options.LocationFormatFunc(link.CallerLocation) + format.MsgStackSep
				stackIndex++
			}
		}
		str += fmt.Sprintf(link.Msg, link.MsgArgs...)
		if len(link.Fields) != 0 {
			str += strings.Repeat(format.PreStackSep, level+1) + format.FieldFormatFunc(link.Fields)
		}
		str += format.ErrorSep
	}
	if format.Options.WithTrace {
		for stackIndex < len(hierarchy.CallerLocations) {
			str += strings.Repeat(format.PreStackSep, level) + format.Options.LocationFormatFunc(hierarchy.CallerLocations[stackIndex]) + format.StackElemSep
			stackIndex++
		}
	}
	if hierarchy.ErrExternal != nil {
		str += strings.Repeat(format.PreStackSep, level) + fmt.Sprint(hierarchy.ErrExternal) + format.ErrorSep
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
