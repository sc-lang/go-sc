# SC language support for Go

The sc package provides support for the Simple Config (SC) language in Go.
It supports decoding SC data into Go values.

## Installation

```
go get github.com/sc-lang/go-sc
```

## Usage

This package works in a similar way to the `encoding/json` package in the standard library.

### Example

Here is an example of decoding some SC data into a Go struct.

```go
package main

import (
	"fmt"

	"github.com/sc-lang/go-sc"
)

var scData = []byte(`{
  name: "foo"
  memory: 256
  required: true
}`)

type Config struct {
    Name       string                 // Implicit key name
    Memory     int    `sc:"memory"`   // Explicit key name
    IsRequired bool   `sc:"required"` // Rename key
}

func main() {
	var config Config
	err := sc.Unmarshal(scData, &config)
	if err != nil {
		fmt.Printf("error: %v\n", err)
	}
	fmt.Printf("%+v\n", config)
}
```

This produces the output:

```
{Name:foo Memory:256 IsRequired:true}
```
