// Copyright (c) 2021 the SC authors. All rights reserved. MIT License.

package scparse

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestNewNumber(t *testing.T) {
	tests := []struct {
		raw     string
		isUint  bool
		isInt   bool
		isFloat bool
		uint64
		int64
		float64
	}{
		{"0", true, true, true, 0, 0, 0},
		{"-0", true, true, true, 0, 0, 0}, // check that -0 is a uint
		{"24", true, true, true, 24, 24, 24},
		{"-42", false, true, true, 0, -42, -42},
		{"0000.1756", false, false, true, 0, 0, 0.1756},
		{"13.79", false, false, true, 0, 0, 13.79},
		{"1E3", true, true, true, 1e3, 1e3, 1e3},
		{"1.5e-3", false, false, true, 0, 0, 1.5e-3},
		{"7e+5", true, true, true, 7e5, 7e5, 7e5},
		{"1e19", true, false, true, 1e19, 0, 1e19},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			n, err := newNumber(Pos{}, tt.raw)
			ok := tt.isUint || tt.isInt || tt.isFloat
			if ok && err != nil {
				t.Fatalf("unexpected error for %q, got %s", tt.raw, err)
			}
			if !ok && err == nil {
				t.Fatalf("want error for %q", tt.raw)
			}
			if tt.isUint {
				if !n.IsUint {
					t.Errorf("want uint for %q", tt.raw)
				}
				if n.Uint64 != tt.uint64 {
					t.Errorf("got uint64 %d, want %d for %q", n.Uint64, tt.uint64, tt.raw)
				}
			} else if n.IsUint {
				t.Errorf("did not want uint for %q", tt.raw)
			}
			if tt.isInt {
				if !n.IsInt {
					t.Errorf("want int for %q", tt.raw)
				}
				if n.Int64 != tt.int64 {
					t.Errorf("got int64 %d, want %d for %q", n.Int64, tt.int64, tt.raw)
				}
			} else if n.IsInt {
				t.Errorf("did not want int for %q", tt.raw)
			}
			if tt.isFloat {
				if !n.IsFloat {
					t.Errorf("want float for %q", tt.raw)
				}
				if n.Float64 != tt.float64 {
					t.Errorf("got float64 %f, want %f for %q", n.Float64, tt.float64, tt.raw)
				}
			} else if n.IsFloat {
				t.Errorf("did not want float for %q", tt.raw)
			}
		})
	}
}

var parseTests = []struct {
	name   string
	input  string
	output string // expected result from calling Format
	ast    *DictionaryNode
}{
	{
		name: "basic",
		input: `{
  name: "service"
  memory: 256
  required: true
}`,
		output: `{
  name: "service"
  memory: 256
  required: true
}
`,
		ast: &DictionaryNode{
			Pos: Pos{1, 1, 0},
			Members: []*MemberNode{
				{
					Pos: Pos{2, 3, 4},
					Key: &IdentifierNode{
						Pos:  Pos{2, 3, 4},
						Name: "name",
					},
					Value: &InterpolatedStringNode{
						Pos: Pos{2, 9, 10},
						Components: []Node{
							&StringNode{
								Pos:   Pos{2, 10, 11},
								Value: "service",
							},
						},
					},
				},
				{
					Pos: Pos{3, 3, 22},
					Key: &IdentifierNode{
						Pos:  Pos{3, 3, 22},
						Name: "memory",
					},
					Value: &NumberNode{
						Pos:     Pos{3, 11, 30},
						IsUint:  true,
						IsInt:   true,
						IsFloat: true,
						Uint64:  256,
						Int64:   256,
						Float64: 256,
						Raw:     "256",
					},
				},
				{
					Pos: Pos{4, 3, 36},
					Key: &IdentifierNode{
						Pos:  Pos{4, 3, 36},
						Name: "required",
					},
					Value: &BoolNode{
						Pos:  Pos{4, 13, 46},
						True: true,
					},
				},
			},
		},
	},
	{
		name:   "single line",
		input:  "{ missing: null, \"yes\": true, `no`: false }",
		output: "{\n  missing: null\n  \"yes\": true\n  `no`: false\n}\n",
		ast: &DictionaryNode{
			Pos: Pos{1, 1, 0},
			Members: []*MemberNode{
				{
					Pos: Pos{1, 3, 2},
					Key: &IdentifierNode{
						Pos:  Pos{1, 3, 2},
						Name: "missing",
					},
					Value: &NullNode{
						Pos: Pos{1, 12, 11},
					},
				},
				{
					Pos: Pos{1, 18, 17},
					Key: &StringNode{
						Pos:   Pos{1, 18, 17},
						Value: "yes",
					},
					Value: &BoolNode{
						Pos:  Pos{1, 25, 24},
						True: true,
					},
				},
				{
					Pos: Pos{1, 31, 30},
					Key: &RawStringNode{
						Pos:   Pos{1, 31, 30},
						Value: "no",
					},
					Value: &BoolNode{
						Pos:  Pos{1, 37, 36},
						True: false,
					},
				},
			},
		},
	},
	{
		name: "variables",
		input: `{
  var: ${value},
  path: "${basepath}/repo",
  image: "golang:${tag}-slim",
}`,
		output: `{
  var: ${value}
  path: "${basepath}/repo"
  image: "golang:${tag}-slim"
}
`,
		ast: &DictionaryNode{
			Pos: Pos{1, 1, 0},
			Members: []*MemberNode{
				{
					Pos: Pos{2, 3, 4},
					Key: &IdentifierNode{
						Pos:  Pos{2, 3, 4},
						Name: "var",
					},
					Value: &VariableNode{
						Pos: Pos{2, 8, 9},
						Identifier: &IdentifierNode{
							Pos:  Pos{2, 10, 11},
							Name: "value",
						},
					},
				},
				{
					Pos: Pos{3, 3, 21},
					Key: &IdentifierNode{
						Pos:  Pos{3, 3, 21},
						Name: "path",
					},
					Value: &InterpolatedStringNode{
						Pos: Pos{3, 9, 27},
						Components: []Node{
							&VariableNode{
								Pos: Pos{3, 10, 28},
								Identifier: &IdentifierNode{
									Pos:  Pos{3, 12, 30},
									Name: "basepath",
								},
							},
							&StringNode{
								Pos:   Pos{3, 21, 39},
								Value: "/repo",
							},
						},
					},
				},
				{
					Pos: Pos{4, 3, 49},
					Key: &IdentifierNode{
						Pos:  Pos{4, 3, 49},
						Name: "image",
					},
					Value: &InterpolatedStringNode{
						Pos: Pos{4, 10, 56},
						Components: []Node{
							&StringNode{
								Pos:   Pos{4, 11, 57},
								Value: "golang:",
							},
							&VariableNode{
								Pos: Pos{4, 18, 64},
								Identifier: &IdentifierNode{
									Pos:  Pos{4, 20, 66},
									Name: "tag",
								},
							},
							&StringNode{
								Pos:   Pos{4, 24, 70},
								Value: "-slim",
							},
						},
					},
				},
			},
		},
	},
	{
		name: "nested structures",
		input: `{
  nums: [1, -2, 3.14],
  map: {
    "state": null,
  }
}`,
		output: `{
  nums: [
    1
    -2
    3.14
  ]
  map: {
    "state": null
  }
}
`,
		ast: &DictionaryNode{
			Pos: Pos{1, 1, 0},
			Members: []*MemberNode{
				{
					Pos: Pos{2, 3, 4},
					Key: &IdentifierNode{
						Pos:  Pos{2, 3, 4},
						Name: "nums",
					},
					Value: &ListNode{
						Pos: Pos{2, 9, 10},
						Elements: []Node{
							&NumberNode{
								Pos:     Pos{2, 10, 11},
								IsUint:  true,
								IsInt:   true,
								IsFloat: true,
								Uint64:  1,
								Int64:   1,
								Float64: 1,
								Raw:     "1",
							},
							&NumberNode{
								Pos:     Pos{2, 13, 14},
								IsUint:  false,
								IsInt:   true,
								IsFloat: true,
								Uint64:  0,
								Int64:   -2,
								Float64: -2,
								Raw:     "-2",
							},
							&NumberNode{
								Pos:     Pos{2, 17, 18},
								IsUint:  false,
								IsInt:   false,
								IsFloat: true,
								Uint64:  0,
								Int64:   0,
								Float64: 3.14,
								Raw:     "3.14",
							},
						},
					},
				},
				{
					Pos: Pos{3, 3, 27},
					Key: &IdentifierNode{
						Pos:  Pos{3, 3, 27},
						Name: "map",
					},
					Value: &DictionaryNode{
						Pos: Pos{3, 8, 32},
						Members: []*MemberNode{
							{
								Pos: Pos{4, 5, 38},
								Key: &StringNode{
									Pos:   Pos{4, 5, 38},
									Value: "state",
								},
								Value: &NullNode{
									Pos: Pos{4, 14, 47},
								},
							},
						},
					},
				},
			},
		},
	},
	{
		name: "comments",
		input: `// config
{ // this belongs to the first member
  required: true, // inline comment
  value: /* this is crazy */ null
  /*
  multiple
  lines
  */
  emptylist: [
    // don't forget me
  ]
  emptydict: {
    /* don't forget me either */
  }
} // inline
// trailing`,
		output: `// config
{
  // this belongs to the first member
  required: true // inline comment
  value:
    /* this is crazy */
    null
  /*
  multiple
  lines
  */
  emptylist: [
    // don't forget me
  ]
  emptydict: {
    /* don't forget me either */
  }
} // inline
// trailing
`,
		ast: &DictionaryNode{
			Pos: Pos{2, 1, 10},
			CommentGroup: CommentGroup{
				Head: []Comment{
					{Pos{1, 1, 0}, " config", false},
				},
				Inline: []Comment{
					{Pos{15, 3, 243}, " inline", false},
				},
				Foot: []Comment{
					{Pos{16, 1, 253}, " trailing", false},
				},
			},
			Members: []*MemberNode{
				{
					Pos: Pos{3, 3, 50},
					Key: &IdentifierNode{
						CommentGroup: CommentGroup{
							Head: []Comment{
								{Pos{2, 3, 12}, " this belongs to the first member", false},
							},
						},
						Pos:  Pos{3, 3, 50},
						Name: "required",
					},
					Value: &BoolNode{
						Pos: Pos{3, 13, 60},
						CommentGroup: CommentGroup{
							Inline: []Comment{
								{Pos{3, 19, 66}, " inline comment", false},
							},
						},
						True: true,
					},
				},
				{
					Pos: Pos{4, 3, 86},
					Key: &IdentifierNode{
						Pos:  Pos{4, 3, 86},
						Name: "value",
					},
					Value: &NullNode{
						CommentGroup: CommentGroup{
							Head: []Comment{
								{Pos{4, 10, 93}, " this is crazy ", true},
							},
						},
						Pos: Pos{4, 30, 113},
					},
				},
				{
					Pos: Pos{9, 3, 149},
					Key: &IdentifierNode{
						Pos: Pos{9, 3, 149},
						CommentGroup: CommentGroup{
							Head: []Comment{
								{Pos{5, 3, 120}, "\n  multiple\n  lines\n  ", true},
							},
						},
						Name: "emptylist",
					},
					Value: &ListNode{
						Pos: Pos{9, 14, 160},
						CommentGroup: CommentGroup{
							Inner: []Comment{
								{Pos{10, 5, 166}, " don't forget me", false},
							},
						},
					},
				},
				{
					Pos: Pos{12, 3, 191},
					Key: &IdentifierNode{
						Pos:  Pos{12, 3, 191},
						Name: "emptydict",
					},
					Value: &DictionaryNode{
						Pos: Pos{12, 14, 202},
						CommentGroup: CommentGroup{
							Inner: []Comment{
								{Pos{13, 5, 208}, " don't forget me either ", true},
							},
						},
					},
				},
			},
		},
	},
}

func TestParse(t *testing.T) {
	for _, tt := range parseTests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("unexpected error %s", err)
			}
			if ok, diff := deepEqual(n, tt.ast); !ok {
				t.Errorf("ASTs not equal:\n%s", diff)
			}
		})
	}
}

func TestPrint(t *testing.T) {
	for _, tt := range parseTests {
		t.Run(tt.name, func(t *testing.T) {
			got := Format(tt.ast)
			if string(got) != tt.output {
				t.Errorf("got formatted SC\n%q\nwant\n%q", got, tt.output)
			}
			// Make sure that if we parse the output we get the same AST (ignoring positions)
			n, err := Parse(got)
			if err != nil {
				t.Fatalf("unexpected error %s", err)
			}
			if ok, diff := deepEqual(n, tt.ast, "Pos"); !ok {
				t.Errorf("ASTs not equal:\n%s", diff)
			}
		})
	}
}

func TestParseError(t *testing.T) {
	tests := []struct {
		name  string
		input string
		err   *Error
	}{
		{
			name:  "top level not a dictionary",
			input: `[1, 2, 3]`,
			err: &Error{
				Pos:     Pos{1, 1, 0},
				Context: "top level value in SC document must be a dictionary",
			},
		},
		{
			name:  "invalid escape char",
			input: `{ foo: "\z" }`,
			err: &Error{
				Pos:     Pos{1, 9, 8},
				Context: `invalid escape character '\z' in string`,
			},
		},
		{
			name: "stuff after document",
			input: `{}
// this comment is fine
{}`,
			err: &Error{
				Pos:     Pos{3, 1, 27},
				Context: "unexpected <{> in end of document",
			},
		},
		{
			name:  "variable in string key",
			input: `{ "foo${bar}": true }`,
			err: &Error{
				Pos:     Pos{1, 7, 6},
				Context: "unexpected <${> in string key, dictionary keys cannot contain variables",
			},
		},
		{
			name:  "invalid key",
			input: `{ 42: null }`,
			err: &Error{
				Pos:     Pos{1, 3, 2},
				Context: `unexpected <Number: "42"> in dictionary key, expected identifier or string`,
			},
		},
		{
			name:  "missing variable",
			input: `{ var: ${} }`,
			err: &Error{
				Pos:     Pos{1, 10, 9},
				Context: "variable name missing after '${'",
			},
		},
		{
			name: "missing colon",
			input: `{
  foo,
  bar,
}`,
			err: &Error{
				Pos:     Pos{2, 6, 7},
				Context: "unexpected <,> in dictionary element, expected ':'",
			},
		},
		{
			name:  "missing comma",
			input: `{ foo: 1 bar: 2 }`,
			err: &Error{
				Pos:     Pos{1, 10, 9},
				Context: `unexpected <Identifier: "bar"> in dictionary, expected ','`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.input))
			if err == nil {
				t.Fatalf("want error")
			}
			var perr *Error
			if !errors.As(err, &perr) {
				t.Fatalf("got err %#v, want *Error", err)
			}
			if perr.Pos != tt.err.Pos {
				t.Errorf("got err pos\n\t%+v\nwant\n\t%+v", perr.Pos, tt.err.Pos)
			}
			if !strings.Contains(perr.Context, tt.err.Context) {
				t.Errorf("got err context\n\t%s\nwant to contain\n\t%s", perr.Context, tt.err.Context)
			}
		})
	}
}

// deepEqual is similar to reflect.DeepEqual but returns a string
// describing the diffs if a and be are not equal.
func deepEqual(a, b interface{}, ignoreFields ...string) (equal bool, diff string) {
	c := comparer{}
	if len(ignoreFields) > 0 {
		c.ignoredFields = make(map[string]bool)
		for _, f := range ignoreFields {
			c.ignoredFields[f] = true
		}
	}
	equal = c.equal(reflect.ValueOf(a), reflect.ValueOf(b))
	if !equal {
		diff = c.buf.String()
	}
	return
}

// comparer holds the state for checking the equality of two values.
type comparer struct {
	stack         []string // parent values for diff
	buf           strings.Builder
	ignoredFields map[string]bool
}

func (c *comparer) equal(a, b reflect.Value) bool {
	if !a.IsValid() && !b.IsValid() {
		// nil == nil
		return true
	}
	if !a.IsValid() && b.IsValid() {
		c.printDiff(a.Type(), "<nil>")
		return false
	}
	if a.IsValid() && !b.IsValid() {
		c.printDiff("<nil>", b.Type())
		return false
	}
	if a.Type() != b.Type() {
		c.printDiff(a.Type(), b.Type())
		return false
	}

	switch a.Kind() {
	case reflect.Array:
		eq := true
		for i := 0; i < a.Len(); i++ {
			c.pushf("array[%d]", i)
			if e := c.equal(a.Index(i), b.Index(i)); !e {
				eq = false
			}
			c.pop()
		}
		return eq
	case reflect.Slice:
		if a.IsNil() && b.IsNil() {
			return true
		}
		if a.IsNil() && !b.IsNil() {
			c.printDiff("<nil slice>", b)
			return false
		}
		if !a.IsNil() && b.IsNil() {
			c.printDiff(a, "<nil slice>")
			return false
		}
		if a.Pointer() == b.Pointer() && a.Len() == b.Len() {
			return true
		}

		al := a.Len()
		bl := b.Len()
		eq := true
		for i := 0; i < al || i < bl; i++ {
			c.pushf("slice[%d]", i)
			if i >= al {
				c.printDiff("<missing>", b.Index(i))
				eq = false
			} else if i >= bl {
				c.printDiff(a.Index(i), "<missing>")
				eq = false
			} else if e := c.equal(a.Index(i), b.Index(i)); !e {
				eq = false
			}
			c.pop()
		}
		return eq
	case reflect.Ptr, reflect.Interface:
		if a.IsNil() && b.IsNil() {
			return true
		}
		if a.IsNil() && !b.IsNil() {
			c.printDiff("<nil>", b.Elem().Type())
			return false
		}
		if !a.IsNil() && b.IsNil() {
			c.printDiff(a.Elem().Type(), "<nil>")
			return false
		}
		return c.equal(a.Elem(), b.Elem())
	case reflect.Map:
		if a.IsNil() && b.IsNil() {
			return true
		}
		if a.IsNil() && !b.IsNil() {
			c.printDiff("<nil map>", b)
			return false
		}
		if !a.IsNil() && b.IsNil() {
			c.printDiff(a, "<nil map>")
			return false
		}
		if a.Pointer() == b.Pointer() {
			return true
		}

		eq := true
		for _, k := range a.MapKeys() {
			c.pushf("map[%v]", k)
			av := a.MapIndex(k)
			bv := b.MapIndex(k)
			if !bv.IsValid() {
				c.printDiff(av, "<missing key>")
				eq = false
			} else if e := c.equal(av, bv); !e {
				eq = false
			}
			c.pop()
		}
		// Check b for keys not in a
		for _, k := range b.MapKeys() {
			// Already handled it in a
			if av := a.MapIndex(k); av.IsValid() {
				continue
			}
			c.pushf("map[%v]", k)
			c.printDiff("<missing key>", b.MapIndex(k))
			eq = false
			c.pop()
		}
		return eq
	case reflect.Struct:
		t := a.Type()
		eq := true
		for i := 0; i < a.NumField(); i++ {
			fname := t.Field(i).Name
			if ok := c.ignoredFields[fname]; ok {
				continue
			}
			c.pushf("%s", fname)
			if e := c.equal(a.Field(i), b.Field(i)); !e {
				eq = false
			}
			c.pop()
		}
		return eq
	case reflect.Func:
		// Best that can be done
		if a.IsNil() == b.IsNil() {
			return true
		}
		if a.IsNil() && !b.IsNil() {
			c.printDiff("<nil func>", b.Type())
			return false
		}
		if !a.IsNil() && b.IsNil() {
			c.printDiff(a.Type(), "<nil func>")
			return false
		}
		c.printDiff(a.Type(), b.Type())
		return false
	}

	// Handle primitives
	// We can't use Interface() because the values might be unexported struct fields
	// which will cause a panic. This is a workaround.
	var ai, bi interface{}
	switch a.Kind() {
	case reflect.Bool:
		ai, bi = a.Bool(), b.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		ai, bi = a.Int(), b.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		ai, bi = a.Uint(), b.Uint()
	case reflect.Float32, reflect.Float64:
		ai, bi = a.Float(), b.Float()
	case reflect.String:
		ai, bi = a.String(), b.String()
	default:
		// Unsupported types; assume not equal
		return false
	}
	if ai != bi {
		c.printDiff(ai, bi)
		return false
	}
	return true
}

func (c *comparer) printDiff(a, b interface{}) {
	if len(c.stack) > 0 {
		c.buf.WriteString(strings.Join(c.stack, "."))
		c.buf.WriteString(": ")
	}
	fmt.Fprintf(&c.buf, "%v != %v", a, b)
	c.buf.WriteByte('\n')
}

func (c *comparer) pushf(format string, args ...interface{}) {
	c.stack = append(c.stack, fmt.Sprintf(format, args...))
}

func (c *comparer) pop() {
	if l := len(c.stack); l > 0 {
		c.stack = c.stack[:l-1]
	}
}
