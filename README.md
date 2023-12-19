# serr

Feature

- call stack
- can wrap error and append message
- structured message support
- can join error
- support depth

Get inspired by

- rotisserie/eris
- sirupsen/logrus
- golang/glog

# Example

## New error

```golang
package main

import (
	"fmt"

	"github.com/MinamiKotoriCute/serr"
)

func f1() error {
	// new error with message
	// return serr.New("run f1 fail")

	// new error with message
	// return serr.Errorf("run %s fail", "f1")

	// new error with structured fields and message
	return serr.Errors(map[string]interface{}{
		"func": "f1",
	}, "run f1 fail")
}

func main() {
	if err := f1(); err != nil {
		fmt.Print(serr.ToString(err, true))
	}
}
```

## Wrap error

```golang
package main

import (
	"fmt"
	"os"

	"github.com/MinamiKotoriCute/serr"
)

func f1() error {
	filename := "file"
	f, err := os.Open(filename)
	if err != nil {
		// wrap err without message
		// return serr.Wrap(err)

		// wrap err with extra message
		// return serr.Wrapf(err, "f1 open file error")

		// wrap err with extra structured fields and message
		return serr.Wraps(err, map[string]interface{}{
			"filename": filename,
		}, "f1 open file error")
	}
	defer f.Close()
	return nil
}

func main() {
	if err := f1(); err != nil {
		fmt.Print(serr.ToString(err, true))
	}
}
```

## Join error

```golang
package main

import (
	"errors"
	"fmt"

	"github.com/MinamiKotoriCute/serr"
)

var (
	Err1 = errors.New("err1")
	Err2 = errors.New("err2")
)

func f1() error {
	return serr.Wrap(Err1)
}

func f2() error {
	return serr.Wrap(Err2)
}

func g() error {
	e1 := f1()
	e2 := f2()

	return serr.Join(e1, e2)
}

func main() {
	if err := g(); err != nil {
		fmt.Println(errors.Is(err, Err1)) // true
		fmt.Println(errors.Is(err, Err2)) // true
		fmt.Print(serr.ToString(err, true))
	}
}
```
