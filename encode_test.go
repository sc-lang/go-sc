// Copyright (c) 2021 the SC authors. All rights reserved. MIT License.

package sc_test

import (
	"encoding"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/sc-lang/go-sc"
	"github.com/sc-lang/go-sc/scparse"
)

func (p *portsConfig) MarshalText() ([]byte, error) {
	s := fmt.Sprintf("%d:%d", p.Src, p.Dst)
	return []byte(s), nil
}

func (p *pathBuf) MarshalSC() (scparse.ValueNode, error) {
	s := strings.Join(p.components, "/")
	c := []scparse.StringContentNode{&scparse.StringNode{Value: s}}
	return &scparse.InterpolatedStringNode{Components: c}, nil
}

// Check that MarshalText is still called even if the receiver is nil
type nilTextMarshaler struct{}

func (m *nilTextMarshaler) MarshalText() ([]byte, error) {
	if m == nil {
		return []byte("nil"), nil
	}
	return []byte("non-nil"), nil
}

// Check that MarshalSC is still called even if the receiver is nil
type nilMarshaler struct{}

func (m *nilMarshaler) MarshalSC() (scparse.ValueNode, error) {
	var s string
	if m == nil {
		s = "nil"
	} else {
		s = "non-nil"
	}
	c := []scparse.StringContentNode{&scparse.StringNode{Value: s}}
	return &scparse.InterpolatedStringNode{Components: c}, nil
}

// Check that a non-string key in a map works if it implements encoding.TextMarshaler
type intTextMarshaler int

func (m intTextMarshaler) MarshalText() ([]byte, error) {
	return []byte(strconv.Itoa(int(m))), nil
}

// Check that a renamed string key in a map still works
type renamedString string

// Check that renamed byte slices still work
type renamedByte byte
type renamedByteSlice []byte
type renamedRenamedByteSlice []renamedByte

func TestMarshal(t *testing.T) {
	tests := []struct {
		name string
		in   interface{}
		want string
	}{
		{
			name: "bool fields",
			in: struct {
				True  bool
				False bool
			}{true, false},
			want: `{
  True: true
  False: false
}
`,
		},
		{
			name: "int fields",
			in: struct {
				Int   int
				Int8  int8
				Int16 int16
				Int32 int32
				Int64 int64
			}{1, -2, 30, 45, -560},
			want: `{
  Int: 1
  Int8: -2
  Int16: 30
  Int32: 45
  Int64: -560
}
`,
		},
		{
			name: "uint fields",
			in: struct {
				Uint   uint
				Uint8  uint8
				Uint16 uint16
				Uint32 uint32
				Uint64 uint64
			}{1, 2, 30, 45, 560},
			want: `{
  Uint: 1
  Uint8: 2
  Uint16: 30
  Uint32: 45
  Uint64: 560
}
`,
		},
		{
			name: "float fields",
			in: struct {
				Float32 float32
				Float64 float64
			}{-1.5, 22.22},
			want: `{
  Float32: -1.5
  Float64: 22.22
}
`,
		},
		{
			name: "string fields",
			in: struct {
				String      string
				RawGoString string
			}{"hello", `multiple
lines`},
			want: `{
  String: "hello"
  RawGoString: "multiple\nlines"
}
`,
		},
		{
			name: "collection values",
			in: struct {
				IntArray [3]int
				StrSlice []string
				Map      map[string]interface{}
			}{
				[3]int{1, 2, 3},
				[]string{"foo", "bar"},
				map[string]interface{}{"a": true, "b": nil},
			},
			want: `{
  IntArray: [
    1
    2
    3
  ]
  StrSlice: [
    "foo"
    "bar"
  ]
  Map: {
    a: true
    b: null
  }
}
`,
		},
		{
			name: "nil values",
			in: struct {
				V   interface{}
				I   *int32
				Sl  []interface{}
				M   map[string]interface{}
				Bsl []byte
				TM  encoding.TextMarshaler
				SCM sc.Marshaler
			}{},
			want: `{
  V: null
  I: null
  Sl: null
  M: null
  Bsl: null
  TM: null
  SCM: null
}
`,
		},
		{
			name: "empty values",
			in: struct {
				B  bool
				I  int
				F  float64
				Sl []interface{}
				M  map[string]interface{}
				St struct{}
			}{Sl: []interface{}{}, M: map[string]interface{}{}},
			want: `{
  B: false
  I: 0
  F: 0
  Sl: []
  M: {}
  St: {}
}
`,
		},
		{
			name: "tag options",
			in: struct {
				I  int `sc:"omitempty"` // named omitempty, not an option
				Io int `sc:"io,omitempty"`
				Ii int `sc:"-"`

				U  uint `sc:"u"`
				Uo uint `sc:"uo,omitempty"`
				Ui uint `sc:"-"`

				F  float64 `sc:"f"`
				Fo float64 `sc:"fo,omitempty"`
				Fi float64 `sc:"-"`

				B  bool `sc:"b"`
				Bo bool `sc:"bo,omitempty"`
				Bi bool `sc:"-"`

				S  string `sc:"s"`
				So string `sc:"so,omitempty"`
				Si string `sc:"-"`

				Sl  []int `sc:"sl"`
				Slo []int `sc:",omitempty"` // no name, just option

				M  map[string]interface{} `sc:"m"`
				Mo map[string]interface{} `sc:"mo,omitempty"`

				St  struct{} `sc:"st"`
				Sto struct{} `sc:",omitempty"`

				P  *int `sc:"p"`
				Po *int `sc:"po,omitempty"`
				Pi *int `sc:"-"`
			}{
				Ii: 10,
				Ui: 255,
				Fi: 12.25,
				Bi: true,
				Si: "foo",
				M:  map[string]interface{}{},
				Mo: map[string]interface{}{},
			},
			want: `{
  omitempty: 0
  u: 0
  f: 0
  b: false
  s: ""
  sl: null
  m: {}
  st: {}
  Sto: {}
  p: null
}
`,
		},
		{
			name: "encoding.TextMarshaler",
			in: struct {
				PortMappings []*portsConfig `sc:"ports"`
			}{[]*portsConfig{
				{Src: 8080, Dst: 8777},
				{Src: 9060, Dst: 3030},
			}},
			want: `{
  ports: [
    "8080:8777"
    "9060:3030"
  ]
}
`,
		},
		{
			name: "Marshaler",
			in: struct {
				IgnorePaths []*pathBuf `sc:"ignoredPaths"`
			}{[]*pathBuf{
				{components: []string{"", "tmp", "foo", "bar"}},
				{components: []string{"src", "deps", "vendor"}},
			}},
			want: `{
  ignoredPaths: [
    "/tmp/foo/bar"
    "src/deps/vendor"
  ]
}
`,
		},
		{
			name: "nil TextMarshaler",
			in: struct {
				V  interface{}
				TM encoding.TextMarshaler
			}{(*nilTextMarshaler)(nil), (*nilTextMarshaler)(nil)},
			want: `{
  V: null
  TM: "nil"
}
`,
		},
		{
			name: "nil Marshaler",
			in: struct {
				V interface{}
				M sc.Marshaler
			}{(*nilMarshaler)(nil), (*nilMarshaler)(nil)},
			want: `{
  V: null
  M: "nil"
}
`,
		},
		{
			name: "embedded structs",
			in: Top{
				Depth0: 1,
				Embed0: Embed0{
					Depth1a: 100,
					Depth1b: 3,
					Depth1c: 4,
					Depth1d: 0xdeadd0,
					Depth1e: 0xdeade0,
				},
				Embed1: &Embed1{
					Depth1a: 2,
					Depth1b: 7,
					Depth1c: -111,
					Depth1d: 0xdeadd1,
					Depth1f: 0xdeadf1,
				},
				embed: embed{
					Num: -21,
				},
			},
			want: `{
  Depth0: 1
  Depth1b: 3
  Depth1c: 4
  Depth1a: 2
  value1b: 7
  Num: -21
}
`,
		},
		{
			name: "embedded struct with named field",
			in: Top2{
				Depth0: 1,
				Embed0: &Embed0{
					Depth1a: 11,
					Depth1b: 12,
					Depth1c: 13,
					Depth1d: 14,
					Depth1e: 15,
				},
			},
			want: `{
  Depth0: 1
  embed: {
    Depth1a: 11
    Depth1b: 12
    Depth1c: 13
    Depth1d: 14
    x: 15
  }
}
`,
		},
		{
			name: "node field",
			in: struct {
				ValueNode  scparse.ValueNode
				NumberNode *scparse.NumberNode
			}{
				&scparse.BoolNode{True: true},
				&scparse.NumberNode{IsInt: true, Int64: 25},
			},
			want: `{
  ValueNode: true
  NumberNode: 25
}
`,
		},
		// This is weird and shouldn't be done, but Unmarshal supports non-pointer nodes as
		// a consequence of how unmarshaling works (because it follows pointers to assign values).
		// Therefore, Marshal should support this for consistency.
		{
			name: "non-pointer nodes",
			in: struct {
				NullNode   scparse.NullNode
				NumberNode scparse.NumberNode
			}{scparse.NullNode{}, scparse.NumberNode{IsInt: true, Int64: 25}},
			want: `{
  NullNode: null
  NumberNode: 25
}
`,
		},
		{
			name: "quoted keys",
			in: map[string]interface{}{
				"multi\nline": true,
				"non-alpha:.": 2,
				"non-ascii🚀":  []int{},
			},
			want: `{
  "multi\nline": true
  "non-alpha:.": 2
  "non-ascii🚀": []
}
`,
		},
		{
			name: "byte slices",
			in: struct {
				B  []byte
				R  renamedByteSlice
				RR renamedRenamedByteSlice
			}{
				[]byte("hello world"),
				renamedByteSlice("bye world"),
				renamedRenamedByteSlice("hello again world"),
			},
			want: `{
  B: "aGVsbG8gd29ybGQ="
  R: "YnllIHdvcmxk"
  RR: "aGVsbG8gYWdhaW4gd29ybGQ="
}
`,
		},
		{
			name: "map with encoding.TextMarshaler keys",
			in: map[intTextMarshaler]interface{}{
				0:  nil,
				2:  "foo",
				-5: true,
			},
			want: `{
  "-5": true
  "0": null
  "2": "foo"
}
`,
		},
		{
			name: "map with custom string type keys",
			in: map[renamedString]interface{}{
				"a": nil,
				"b": "foo",
				"c": true,
			},
			want: `{
  a: null
  b: "foo"
  c: true
}
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := sc.Marshal(tt.in)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			got := string(b)
			if got != tt.want {
				t.Errorf("got marshaled value\n\t%#v\nwant\n\t%#v", got, tt.want)
			}
		})
	}
}

func TestMarshalError(t *testing.T) {
	tests := []struct {
		name string
		in   interface{}
		want string
	}{
		{
			name: "unsupported type",
			in:   make(chan bool),
			want: "sc: unsupported type: chan bool",
		},
		{
			name: "not a map or struct",
			in:   []interface{}{true, 2, "foo"},
			want: "sc: unsupported type: []interface {}",
		},
		{
			name: "not a ValueNode",
			in: struct {
				Node *scparse.IdentifierNode
			}{&scparse.IdentifierNode{Name: "key"}},
			want: "sc: unsupported node type *scparse.IdentifierNode; not a value node",
		},
		{
			name: "map with non-string keys",
			in: map[int]interface{}{
				0:  nil,
				2:  "foo",
				-5: true,
			},
			want: "sc: unsupported map with key type: int",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sc.Marshal(tt.in)
			if err == nil {
				t.Fatalf("want error")
			}
			var merr *sc.MarshalError
			if !errors.As(err, &merr) {
				t.Errorf("got error of type %T, want %T", err, merr)
			}
			if err.Error() != tt.want {
				t.Errorf("got error string\n\t%s\nwant\n\t%s", err, tt.want)
			}
		})
	}
}

type marshalPanic struct{}

func (marshalPanic) MarshalSC() (scparse.ValueNode, error) {
	panic(0xdead)
}

// Test that panics not caused by sc are not recovered
func TestMarshalPanic(t *testing.T) {
	defer func() {
		if got := recover(); got != 0xdead {
			t.Errorf("got panic value %v (%T), want 0xdead", got, got)
		}
	}()
	_, _ = sc.Marshal(&marshalPanic{})
	t.Error("want Marshal to panic")
}
