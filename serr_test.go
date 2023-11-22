package serr

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

var ErrSomething = errors.New("something")
var ErrAnother = errors.New("another")

func f1(hasError bool) error {
	if hasError {
		return Wrapf(ErrSomething, map[string]interface{}{"f": "f1"}, "f1")
	}

	return nil
}

func f2(hasError bool) error {
	if err := f1(hasError); err != nil {
		return Wrapf(Wrapf(err, map[string]interface{}{"f": "f2", "order": 2}, "f2-2"), map[string]interface{}{"f": "f2", "order": 1}, "f2-1")
	}

	return nil
}

func f3(hasError bool) error {
	e1 := f1(hasError)

	if err := f2(hasError); err != nil {
		return Join(err, ErrAnother, e1)
	}

	return nil
}

func f4(hasError bool) error {
	if err := f3(hasError); err != nil {
		return Wrapf(err, map[string]interface{}{"f": "f4"}, "f4")
	}

	return nil
}

func f5(hasError bool) error {
	if err := f4(hasError); err != nil {
		return Join(nil, err, nil)
	}

	return nil
}

func TestSerr(t *testing.T) {
	err := f5(true)

	if !errors.Is(err, ErrSomething) {
		t.Fail()
	}
	if !errors.Is(err, ErrAnother) {
		t.Fail()
	}

	f := ToJSON(err, true)
	b, _ := json.MarshalIndent(f, "", "  ")

	fmt.Print(string(b))
}

func TestSerr2(t *testing.T) {
	err := f5(true)

	if !errors.Is(err, ErrSomething) {
		t.Fail()
	}
	if !errors.Is(err, ErrAnother) {
		t.Fail()
	}

	fmt.Print(ToString(err, true))
}
