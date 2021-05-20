// Copyright (c) 2021 the SC authors. All rights reserved. MIT License.

// This file contains the public sc API and documentation.

// Package sc implements support for the SC language in Go.
// It allows for decoding SC data into Go values.
package sc

import (
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/sc-lang/go-sc/scparse"
)

///// Unmarshaling & Decoding /////

// Unmarshal parses the SC-encoded data and stores the result in the value pointed to by v.
// If v is nil or not a pointer, Unmarshal returns InvalidUnmarshalError.
// The top level SC value must be a dictionary. Therefore, v must point to either a map or
// a struct that is capable of holding the SC data.
//
// Unmarshal will initialize any nested maps, slices, and pointers it encounters as needed.
// If a value implements the Unmarshaler interface, Unmarshal calls its UnmarshalSC method
// with the SC node and any variable values provided to Unmarshal. If the value implements
// the encoding.TextUnmarshaler and the SC value is a string, Unmarshal call the value's
// UnmarshalText method with the unquoted form of the SC string.
//
// Struct fields are only unmarshaled if they are exported and are unmarshaled using the
// field name as the default key. Custom keys may be defined via the "sc" name
// in the field tag.
//
// Unmarshal supports unmarshaling into node types defined in the scparse package.
// This can allow for delaying the unmarshaling process and for accessing parts of the
// node like comments. UnmarshalNode may be used to continue the unmarshaling process and
// unmarshal the node into a Go value.
//
// If the target value is an empty interface, Unmarshal stores SC values into the following Go values:
//
//  nil for SC null
//  bool for SC booleans
//  int for SC numbers that can be represented as integers
//  float64 for all other SC numbers
//  []interface{} for SC lists
//  map[string]interface{} for SC dictionaries
//
// If a SC value is not appropriate for a given target type, Umarshal skips that field and
// completes the unmarshaling as best it can. Unmarshal keeps track of all non-critical
// errors encountered and returns a sc.Errors which is a list of error values that can be
// checked individually. If an error is encountered while parsing the SC value,
// Unmarshal will immediately return the parse error, which will likely be a *scparse.Error.
//
// Unmarshal can optionally be provided additional option arguments that modify the unmarshal process.
// For example, sc.WithVariables can be used to provide values for SC variables that will be expanded
// during unmarshaling. See the documentation for each UnmarshalOption to learn more.
func Unmarshal(data []byte, v interface{}, opts ...UnmarshalOption) error {
	n, err := scparse.Parse(data)
	if err != nil {
		return err
	}

	var d decoder
	for _, opt := range opts {
		opt(&d)
	}
	return d.unmarshal(n, v)
}

// UnmarshalNode is like Unmarshal but it takes a ValueNode instead of SC-encoded data.
//
// If an SC value was previously unmarshaled into a node, UnmarshalNode can be used
// to unmarshal the node into a Go value.
//
// See the documentation for Unmarshal for details on the unmarshal process.
func UnmarshalNode(n scparse.ValueNode, v interface{}, opts ...UnmarshalOption) error {
	var d decoder
	for _, opt := range opts {
		opt(&d)
	}
	return d.unmarshal(n, v)
}

// UnmarshalOption is an option that can be provided to Unmarshal to customize
// behaviour during the unmarshaling process.
//
// The signature contains an unexported type so that only options defined in this
// package are valid.
type UnmarshalOption func(*decoder)

// WithVariables sets the variables that should be used during unmarshaling.
func WithVariables(vars Variables) UnmarshalOption {
	return func(d *decoder) {
		d.vars = vars
	}
}

// WithDisallowUnknownFields controls how Unmarshal will behave when the destination
// is a struct and the input contains dictionary keys which do not match any
// non-ignored, exported fields in the destination.
//
// By default, unknown fields are silently ignored. If set to true,
// unknown fields will instead cause an error to be returned during unmarshaling.
func WithDisallowUnknownFields(b bool) UnmarshalOption {
	return func(d *decoder) {
		d.disallowUnknownFields = b
	}
}

// TODO(@cszatmary): Does it make sense to allow unknown variables by default?
// Should it be the other way around?

// WithDisallowUnknownVariables controls how Unmarshal will behave when a variable is being
// decoded and no matching variable value is found.
//
// By default, unknown variables are silently ignored. If set to true,
// unknown variables will instead cause an UnmarshalUnknownVariableError
// to be returned during unmarshaling.
//
// How variables are ignored depends on where the variable occurs in the SC source.
// If the variable is a standalone value, the zero value of the destination Go value
// will be used. If the variable is interpolated in a string, it will be treated as an empty string.
func WithDisallowUnknownVariables(b bool) UnmarshalOption {
	return func(d *decoder) {
		d.disallowUnknownVars = b
	}
}

// Unmarshaler is the interface implemented by types that can unmarshal
// a SC description of themselves. This can be used to customize the unmarshaling
// process for a type.
type Unmarshaler interface {
	UnmarshalSC(scparse.ValueNode, Variables) error
}

// TODO(@cszatmary): Does it make sense to have Decoder?
// It doesn't have the same behaviour as other decoders, ex json.Decoder,
// because it reads everything from the reader. Maybe we should replace
// this with a UnmarshalReader function instead?

// A Decoder reads and decodes SC values from an input stream.
type Decoder struct {
	r io.Reader
	d decoder
}

// NewDecoder returns a new decoder that reads from r.
//
// The decoder will read the entire contents of r and expects r to
// only contain valid SC data.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: r}
}

// Variables sets the variables that should be used during decoding.
func (dec *Decoder) Variables(vars Variables) {
	dec.d.vars = vars
}

// DisallowUnknownFields controls how the Decoder will behave when the destination
// is a struct and the input contains dictionary keys which do not match any
// non-ignored, exported fields in the destination.
//
// By default, unknown fields are silently ignored. If set to true,
// unknown fields will instead cause an error to be returned during decoding.
func (dec *Decoder) DisallowUnknownFields(b bool) {
	dec.d.disallowUnknownFields = b
}

// DisallowUnknownVariables controls how the Decoder will behave when a variable is being
// decoded and no matching variable value is found.
//
// By default, unknown variables are silently ignored. If set to true,
// unknown variables will instead cause an UnmarshalUnknownVariableError
// to be returned during decoding.
//
// How variables are ignored depends on where the variable occurs in the SC source.
// If the variable is a standalone value, the zero value of the destination Go value
// will be used. If the variable is interpolated in a string, it will be treated as an empty string.
func (dec *Decoder) DisallowUnknownVariables(b bool) {
	dec.d.disallowUnknownVars = b
}

// Decode reads the SC-encoded value from its input and stores it in the value pointed to by v.
//
// See the documentation for Unmarshal for details about the decoding process.
func (dec *Decoder) Decode(v interface{}) error {
	data, err := io.ReadAll(dec.r)
	if err != nil {
		return err
	}
	n, err := scparse.Parse(data)
	if err != nil {
		return err
	}
	return dec.d.unmarshal(n, v)
}

// UnmarshalTypeError describes a SC value that was not
// appropriate for a value of a specified Go type.
type UnmarshalTypeError struct {
	NodeType scparse.NodeType // Type of the AST node.
	Type     reflect.Type     // Type of Go value.
	Pos      scparse.Pos      // Position of the SC node in the input text.
	Struct   string           // Name of the struct type containing the field.
	Field    string           // The full path from the root struct to the field.
}

func (e *UnmarshalTypeError) Error() string {
	if e.Struct != "" || e.Field != "" {
		return fmt.Sprintf("sc: cannot unmarshal %s into Go struct field %s.%s of type %s", e.NodeType, e.Struct, e.Field, e.Type.String())
	}
	return fmt.Sprintf("sc: cannot unmarshal %s into Go value of type %s", e.NodeType, e.Type.String())
}

// UnmarshalUnknownVariableError describes a SC variable that did not have an
// associated value during unmarshaling.
type UnmarshalUnknownVariableError struct {
	Variable string      // The name of the variable.
	Pos      scparse.Pos // Position of the SC node in the input text.
}

func (e *UnmarshalUnknownVariableError) Error() string {
	return fmt.Sprintf("sc: unknown variable %q", e.Variable)
}

// InvalidUnmarshalError describes an invalid argument passed to Unmarshal.
// (The argument to Unmarshal must be a non-nil pointer.)
type InvalidUnmarshalError struct {
	Type reflect.Type // Type that was passed to Unmarshal.
}

func (e *InvalidUnmarshalError) Error() string {
	if e.Type == nil {
		return "sc: Unmarshal(nil)"
	}
	if e.Type.Kind() != reflect.Ptr {
		return fmt.Sprintf("sc: Unmarshal(non-pointer %s)", e.Type.String())
	}
	return fmt.Sprintf("sc: Unmarshal(nil %s)", e.Type.String())
}

// Errors is a list of errors that occurred during unmarshaling.
//
// Where possible, Unmarshal tries to continue unmarshaling as best it can
// and report all encountered errors. This way multiple errors can be discovered
// at once.
//
// Each error can be inspected individually to obtain more details about it.
type Errors []error

func (e Errors) Error() string {
	var sb strings.Builder
	for i, err := range e {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(err.Error())
	}
	return sb.String()
}

// Variables represents a set of variables provided during the unmarshaling process.
// It allows for looking up a variable value from a VariableNode.
//
// The zero value is a valid Variables instance and represents an empty set of variables.
type Variables struct {
	// The value that contains the variables.
	// This must be map[T]any where T.Kind() == string.
	v reflect.Value
	// The type of the map key. Cached for easy conversion during lookups.
	kt reflect.Type
}

// TODO(@cszatmary): A possible alternative to requiring NewVariables/MustVariables
// is to change the definition of UnmarshalOption to return an error.
// Then options (like WithVariables) could return an error which Unmarshal would
// check when calling them. Something to consider as it would mean less API.
// The downside is it would not be as clear if the an error was caused by incorrect variables.

// NewVariables creates a new Variables instance using the variable values v.
// v must be a map whose keys are a string type.
//
// If v is not a valid type, an error will be returned.
// NewVariables(nil) returns the zero value.
func NewVariables(v interface{}) (Variables, error) {
	if v == nil {
		return Variables{}, nil
	}
	vv := reflect.ValueOf(v)
	if vv.Kind() != reflect.Map {
		return Variables{}, fmt.Errorf("sc: invalid type %T used for variables", v)
	}
	// Check that they key is a string type
	kt := vv.Type().Key()
	if kt.Kind() != reflect.String {
		return Variables{}, fmt.Errorf("sc: invalid key type %s in variables map", kt)
	}
	return Variables{v: vv, kt: kt}, nil
}

// MustVariables is like NewVariables but panics if v is an invalid type.
func MustVariables(v interface{}) Variables {
	vars, err := NewVariables(v)
	if err != nil {
		panic(err)
	}
	return vars
}

func (vars Variables) lookup(n *scparse.VariableNode) reflect.Value {
	// Handle zero value vars
	if !vars.v.IsValid() {
		return reflect.Value{}
	}
	kv := reflect.ValueOf(n.Identifier.Name)
	return vars.v.MapIndex(kv.Convert(vars.kt))
}

// Lookup finds the variable value matching n if it exists.
// The second return value can be used to check if the variable was found.
func (vars Variables) Lookup(n *scparse.VariableNode) (interface{}, bool) {
	v := vars.lookup(n)
	if !v.IsValid() {
		return nil, false
	}
	return v.Interface(), true
}

///// Marshaling & Encoding /////

// Marshal encodes v into an SC representation and returns the data.
// v must be a struct or a map or a pointer to one of these types.
//
// If a value implements the Marshaler interface, Marshal calls its
// MarshalSC method to produce an SC node. If the value implements
// the encoding.TextMarshaler interface, Marshal calls its MarshalText
// method and encodes the result as an SC string.
//
// Struct fields are only encoded if they are exported and use the field
// name as the key by default. Struct fields can be customized
// using the "sc" key in the field's tag. The tag value is the name of the field
// optionally followed by a comma-separated list of options. The name of the field
// can be omitted to specify options without overridding the default field name.
// If the tag value is "-", then the field will be omitted.
//
// The "omitempty" option causes the field to be omitted if it is an empty value.
// Empty values are false, 0, a nil pointer, a nil interface value,
// and an empty array, slice, map, or string.
func Marshal(v interface{}) ([]byte, error) {
	var e encoder
	n, err := e.marshal(v)
	if err != nil {
		return nil, err
	}
	return scparse.Format(n), nil
}

// Marshaler is the interface implemented by types that can marshal
// themselves into an SC value.
type Marshaler interface {
	MarshalSC() (scparse.ValueNode, error)
}

// MarshalError is returned by Marshal and describes an error that occurred
// during marshaling.
type MarshalError struct {
	Value   reflect.Value // The value that caused the error.
	Context string        // The details of the error.
}

func (e *MarshalError) Error() string {
	return "sc: " + e.Context
}
