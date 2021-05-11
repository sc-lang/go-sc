// Copyright (c) 2021 the SC authors. All rights reserved. MIT License.

package sc_test

import (
	"bytes"
	"encoding"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/sc-lang/go-sc"
	"github.com/sc-lang/go-sc/scparse"
)

// encoding.TextUnmarshaler implementation

type portsConfig struct {
	Src, Dst int
}

func (p *portsConfig) UnmarshalText(b []byte) error {
	i := bytes.IndexByte(b, ':')
	if i == -1 {
		return errors.New("missing :")
	}
	var err error
	p.Src, err = strconv.Atoi(string(b[:i]))
	if err != nil {
		return err
	}
	p.Dst, err = strconv.Atoi(string(b[i+1:]))
	if err != nil {
		return err
	}
	return nil
}

var _ encoding.TextUnmarshaler = (*portsConfig)(nil)

// Unmarshaler implementation

type pathBuf struct {
	components []string
}

func (p *pathBuf) UnmarshalSC(n scparse.Node, vars sc.Variables) error {
	var s string
	switch n := n.(type) {
	case *scparse.InterpolatedStringNode:
		if err := sc.UnmarshalNode(n, &s, sc.WithVariables(vars)); err != nil {
			return err
		}
	case *scparse.RawStringNode:
		s = n.Value
	default:
		return &sc.UnmarshalTypeError{
			NodeType: n.Type(),
			Type:     reflect.TypeOf(pathBuf{}),
			Pos:      n.Position(),
		}
	}
	p.components = strings.Split(s, "/")
	return nil
}

var _ sc.Unmarshaler = (*pathBuf)(nil)

type withTextUnmarshaler struct {
	PortMappings []portsConfig `sc:"ports"`
}

type withUnmarshaler struct {
	IgnorePaths []pathBuf `sc:"ignoredPaths"`
}

// Used to test embedding

type Top struct {
	Depth0 int
	Embed0
	*Embed1
	embed // Has exported field
}

type Top2 struct {
	Depth0  int
	*Embed0 `sc:"embed"` // Named embedded struct
}

type Embed0 struct {
	Depth1a int // Overridden by Embed1
	Depth1b int // Used since Embed1 renames it
	Depth1c int // Used since Embed1 ignores it
	Depth1d int // Cancelled out by Embed1
	Depth1e int `sc:"x"` // Cancelled out by Embed1
}

type Embed1 struct {
	Depth1a int `sc:"Depth1a"`
	Depth1b int `sc:"value1b"`
	Depth1c int `sc:"-"`
	Depth1d int
	Depth1f int `sc:"x"`
}

type embed struct {
	Num int
}

// Test number possibilities

type nums struct {
	Int     int
	Int8    int8
	Uint    uint
	Uint16  uint16
	Float32 float32
	Float64 float64
}

// Types for variables

type Key string

type Data map[string]interface{}

func TestUnmarshal(t *testing.T) {
	tests := []struct {
		name  string
		input string
		vars  interface{}
		v     interface{}
		want  interface{}
	}{
		{
			name:  "number types",
			input: `{ int: -234, int8: -35, uint: 1239807320, uint16: 256, float32: 0.33, float64: 56.789 }`,
			v:     &nums{},
			want:  &nums{-234, -35, 1239807320, 256, 0.33, 56.789},
		},
		{
			name: "interface{}",
			input: `{ none: null, num1: 10, num2: 12.5, yes: true, no: false, str: "hello",
				vals: [1, true, "foo"], map: { a: 1, b: "yes", c: null }}`,
			v: &map[string]interface{}{},
			want: &map[string]interface{}{
				"none": nil, "num1": int(10), "num2": float64(12.5), "yes": true, "no": false,
				"str": "hello", "vals": []interface{}{int(1), true, "foo"},
				"map": map[string]interface{}{"a": int(1), "b": "yes", "c": nil},
			},
		},
		{
			name: "embeddeding",
			input: `{
				Depth0: 1
				Depth1a: 2
				Depth1b: 3
				Depth1c: 4
				Depth1d: 5
				x: 6
				value1b: 7
				Num: -21

			}`,
			v: &Top{},
			want: &Top{
				Depth0: 1,
				Embed0: Embed0{
					Depth1b: 3,
					Depth1c: 4,
				},
				Embed1: &Embed1{
					Depth1a: 2,
					Depth1b: 7,
				},
				embed: embed{
					Num: -21,
				},
			},
		},
		{
			name: "embedding named field",
			input: `{
				Depth0: 1
				embed: {
					Depth1a: 11
					Depth1b: 12
					Depth1c: 13
					Depth1d: 14
					x: 15
				}
			}`,
			v: &Top2{},
			want: &Top2{
				Depth0: 1,
				Embed0: &Embed0{
					Depth1a: 11,
					Depth1b: 12,
					Depth1c: 13,
					Depth1d: 14,
					Depth1e: 15,
				},
			},
		},
		{
			name: "encoding.TextUnmarshaler",
			input: `{
				ports: [
					"8080:8777"
					` + "`9060:3030`" + `
				]
			}`,
			v: &withTextUnmarshaler{},
			want: &withTextUnmarshaler{
				PortMappings: []portsConfig{
					{Src: 8080, Dst: 8777},
					{Src: 9060, Dst: 3030},
				},
			},
		},
		{
			name: "Unmarshaler",
			input: `{
				ignoredPaths: [
					"/tmp/foo/bar"
					` + "`src/deps/vendor`" + `
				]
			}`,
			v: &withUnmarshaler{},
			want: &withUnmarshaler{
				IgnorePaths: []pathBuf{
					{components: []string{"", "tmp", "foo", "bar"}},
					{components: []string{"src", "deps", "vendor"}},
				},
			},
		},
		{
			name: "variables: custom map key",
			input: `{
				num: ${magic}
				x: ${y}
				path: "/foo/${dir}/baz"
			}`,
			vars: map[Key]interface{}{"magic": 3, "y": "z", "dir": "bin/bar"},
			v:    &map[string]interface{}{},
			want: &map[string]interface{}{"num": 3, "x": "z", "path": "/foo/bin/bar/baz"},
		},
		{
			name: "variables: custom map type",
			input: `{
				num: ${magic}
				x: ${y}
				path: "/foo/${dir}/baz"
			}`,
			vars: Data{"magic": 3, "y": "z", "dir": "bin/bar"},
			v:    &map[string]interface{}{},
			want: &map[string]interface{}{"num": 3, "x": "z", "path": "/foo/bin/bar/baz"},
		},
		{
			name: "variables: missing",
			input: `{
				code: ${id}
				title: ${name}
				path: "/foo/${dir}/baz"
			}`,
			v: &struct {
				Code  int
				Title string
				Path  string
			}{},
			want: &struct {
				Code  int
				Title string
				Path  string
			}{0, "", "/foo//baz"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vars, err := sc.NewVariables(tt.vars)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			err = sc.Unmarshal([]byte(tt.input), tt.v, sc.WithVariables(vars))
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if !reflect.DeepEqual(tt.v, tt.want) {
				t.Errorf("got unmarshaled value\n\t%#v\nwant\n\t%#v", tt.v, tt.want)
			}
		})
	}
}

func TestInvalidUnmarshalError(t *testing.T) {
	tests := []struct {
		name string
		v    interface{}
		err  sc.InvalidUnmarshalError
		text string
	}{
		{
			name: "nil",
			v:    nil,
			err:  sc.InvalidUnmarshalError{Type: nil},
			text: "sc: Unmarshal(nil)",
		},
		{
			name: "non-pointer",
			v:    Top{},
			err:  sc.InvalidUnmarshalError{Type: reflect.TypeOf(Top{})},
			text: "sc: Unmarshal(non-pointer sc_test.Top)",
		},
		{
			name: "nil-pointer",
			v:    (*Top)(nil),
			err:  sc.InvalidUnmarshalError{Type: reflect.TypeOf((*Top)(nil))},
			text: "sc: Unmarshal(nil *sc_test.Top)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sc.Unmarshal([]byte(`{}`), tt.v)
			if err == nil {
				t.Fatalf("want error")
			}
			var invalidErr *sc.InvalidUnmarshalError
			if !errors.As(err, &invalidErr) {
				t.Fatalf("got error of type %T, want %T", err, invalidErr)
			}
			if *invalidErr != tt.err {
				t.Errorf("got error\n\t%+v\nwant\n\t%+v", *invalidErr, tt.err)
			}
			if err.Error() != tt.text {
				t.Errorf("got error string\n\t%s\nwant\n\t%s", err, tt.text)
			}
		})
	}
}

func TestUnmarshalErrors(t *testing.T) {
	type V struct {
		FieldA string
		FieldB int
		FieldC bool
		FieldD float64
		FieldE string
	}
	err := sc.Unmarshal([]byte(`{
		FieldA: "foo"
		FieldB: 12.5
		FieldC: 1
		FieldD: ${num}
		FieldE: "foo-${x}-bar"
		FieldF: true
	}`), &V{}, sc.WithDisallowUnknownFields(true), sc.WithDisallowUnknownVariables(true))
	if err == nil {
		t.Fatalf("want error")
	}
	var errs sc.Errors
	if !errors.As(err, &errs) {
		t.Fatalf("got error of type %T, want Errors", err)
	}
	if len(errs) != 5 {
		t.Fatalf("got %d errors, want %d", len(errs), 5)
	}

	// Check each error
	// 1
	var typeErr *sc.UnmarshalTypeError
	if !errors.As(errs[0], &typeErr) {
		t.Fatalf("got error of type %T, want %T", errs[0], typeErr)
	}
	wantTypeErr := sc.UnmarshalTypeError{
		NodeType: scparse.NodeNumber,
		Type:     reflect.TypeOf(int(0)),
		Pos:      scparse.Pos{Line: 3, Column: 11, Byte: 28},
		Struct:   "V",
		Field:    "FieldB",
	}
	if *typeErr != wantTypeErr {
		t.Errorf("got error\n\t%+v\nwant\n\t%+v", *typeErr, wantTypeErr)
	}
	// 2
	if !errors.As(errs[1], &typeErr) {
		t.Fatalf("got error of type %T, want %T", errs[1], typeErr)
	}
	wantTypeErr = sc.UnmarshalTypeError{
		NodeType: scparse.NodeNumber,
		Type:     reflect.TypeOf(false),
		Pos:      scparse.Pos{Line: 4, Column: 11, Byte: 43},
		Struct:   "V",
		Field:    "FieldC",
	}
	if *typeErr != wantTypeErr {
		t.Errorf("got error\n\t%+v\nwant\n\t%+v", *typeErr, wantTypeErr)
	}
	// 3
	var unknownVarErr *sc.UnmarshalUnknownVariableError
	if !errors.As(errs[2], &unknownVarErr) {
		t.Fatalf("got error of type %T, want %T", errs[2], unknownVarErr)
	}
	wantUnknownVarErr := sc.UnmarshalUnknownVariableError{
		Variable: "num",
		Pos:      scparse.Pos{Line: 5, Column: 11, Byte: 55},
	}
	if *unknownVarErr != wantUnknownVarErr {
		t.Errorf("got error\n\t%+v\nwant\n\t%+v", *unknownVarErr, wantUnknownVarErr)
	}
	// 4
	if !errors.As(errs[3], &unknownVarErr) {
		t.Fatalf("got error of type %T, want %T", errs[3], unknownVarErr)
	}
	wantUnknownVarErr = sc.UnmarshalUnknownVariableError{
		Variable: "x",
		Pos:      scparse.Pos{Line: 6, Column: 16, Byte: 77},
	}
	if *unknownVarErr != wantUnknownVarErr {
		t.Errorf("got error\n\t%+v\nwant\n\t%+v", *unknownVarErr, wantUnknownVarErr)
	}

	wantText := `sc: cannot unmarshal Number into Go struct field V.FieldB of type int
sc: cannot unmarshal Number into Go struct field V.FieldC of type bool
sc: unknown variable "num"
sc: unknown variable "x"
sc: unknown field "FieldF"`
	if err.Error() != wantText {
		t.Errorf("got error string\n\t%s\nwant\n\t%s", err, wantText)
	}
}

func TestUnmarshalParseError(t *testing.T) {
	err := sc.Unmarshal([]byte(`{ var: ${} }`), &map[string]interface{}{})
	if err == nil {
		t.Fatalf("want error")
	}
	var parseErr *scparse.Error
	if !errors.As(err, &parseErr) {
		t.Errorf("got error of type %T, want *scparse.Error", err)
	}
	// Just check that an *scparse.Error is returned.
	// scparse has tests to check the error contents, we can assume it is correct here.
}
