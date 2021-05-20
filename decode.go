// Copyright (c) 2021 the SC authors. All rights reserved. MIT License.

package sc

import (
	"encoding"
	"encoding/base64"
	"fmt"
	"reflect"
	"strings"

	"github.com/sc-lang/go-sc/scparse"
)

var (
	nodeType            = reflect.TypeOf((*scparse.Node)(nil)).Elem()
	valueNodeType       = reflect.TypeOf((*scparse.ValueNode)(nil)).Elem()
	textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
)

// A large amount of the reflection code in this file is adapted from
// encoding/json because reflection is not fun.
// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

func newUnmarshalTypeError(n scparse.Node, t reflect.Type) *UnmarshalTypeError {
	return &UnmarshalTypeError{NodeType: n.Type(), Type: t, Pos: n.Position()}
}

// decoder decodes a Node into a Go type.
type decoder struct {
	errorContext struct { // provides context for type errors
		Struct     reflect.Type
		FieldStack []string
	}
	errors                Errors
	vars                  Variables
	disallowUnknownFields bool
	disallowUnknownVars   bool
}

// saveError saves err by adding it to the list of errors.
// It will add context to the error with information from d.errorContext.
func (d *decoder) saveError(err error) {
	if d.errorContext.Struct != nil || len(d.errorContext.FieldStack) > 0 {
		switch err := err.(type) {
		case *UnmarshalTypeError:
			err.Struct = d.errorContext.Struct.Name()
			err.Field = strings.Join(d.errorContext.FieldStack, ".")
		}
	}
	d.errors = append(d.errors, err)
}

func (d *decoder) unmarshal(n scparse.ValueNode, v interface{}) error {
	rv := reflect.ValueOf(v)
	// v must be a pointer and not nil
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return &InvalidUnmarshalError{reflect.TypeOf(v)}
	}

	// Decode rv not rv.Elem because the Unmarshaler interface test
	// must be applied at the top level of the value.
	err := d.decodeValue(n, rv)
	if err != nil {
		d.saveError(err)
	}
	if len(d.errors) > 0 {
		return d.errors
	}
	return nil
}

func (d *decoder) decodeValue(n scparse.ValueNode, v reflect.Value) error {
	// If v can't be set just ignore it
	if !v.IsValid() {
		return nil
	}

	switch n := n.(type) {
	case *scparse.NullNode:
		return d.decodeNull(n, v)
	case *scparse.BoolNode:
		return d.decodeBool(n, v)
	case *scparse.NumberNode:
		return d.decodeNumber(n, v)
	case *scparse.InterpolatedStringNode:
		return d.decodeInterpolatedString(n, v)
	case *scparse.RawStringNode:
		return d.decodeRawString(n, v)
	case *scparse.VariableNode:
		return d.decodeVariable(n, v)
	case *scparse.DictionaryNode:
		return d.decodeDictionary(n, v)
	case *scparse.ListNode:
		return d.decodeList(n, v)
	default:
		panic(fmt.Errorf("impossible: invalid node type used as value: %T", n))
	}
}

func (d *decoder) decodeNull(n *scparse.NullNode, v reflect.Value) error {
	// Check for unmarshaler.
	u, ut, pv := indirect(v, true)
	if u != nil {
		return u.UnmarshalSC(n, d.vars)
	}
	if ut != nil {
		d.saveError(newUnmarshalTypeError(n, v.Type()))
		return nil
	}

	v = pv
	switch v.Kind() {
	case reflect.Interface:
		if t := v.Type(); t == nodeType || t == valueNodeType {
			v.Set(reflect.ValueOf(n))
			break
		}
		fallthrough // Handle nil assignment below
	case reflect.Ptr, reflect.Map, reflect.Slice:
		v.Set(reflect.Zero(v.Type()))
	case reflect.Struct:
		if v.Type() == reflect.TypeOf((*scparse.NullNode)(nil)).Elem() {
			v.Set(reflect.ValueOf(n).Elem())
			break
		}
		// otherwise ignore null, the zero value will be used
	}
	return nil
}

func (d *decoder) decodeBool(n *scparse.BoolNode, v reflect.Value) error {
	// Check for unmarshaler.
	u, ut, pv := indirect(v, false)
	if u != nil {
		return u.UnmarshalSC(n, d.vars)
	}
	if ut != nil {
		d.saveError(newUnmarshalTypeError(n, v.Type()))
		return nil
	}

	v = pv
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(n.True)
	case reflect.Interface:
		if t := v.Type(); t == nodeType || t == valueNodeType {
			v.Set(reflect.ValueOf(n))
			break
		}
		if v.NumMethod() == 0 {
			v.Set(reflect.ValueOf(n.True))
			break
		}
		d.saveError(newUnmarshalTypeError(n, v.Type()))
	case reflect.Struct:
		if v.Type() == reflect.TypeOf((*scparse.BoolNode)(nil)).Elem() {
			v.Set(reflect.ValueOf(n).Elem())
			break
		}
		fallthrough
	default:
		d.saveError(newUnmarshalTypeError(n, v.Type()))
	}
	return nil
}

func (d *decoder) decodeNumber(n *scparse.NumberNode, v reflect.Value) error {
	// Check for unmarshaler.
	u, ut, pv := indirect(v, false)
	if u != nil {
		return u.UnmarshalSC(n, d.vars)
	}
	if ut != nil {
		d.saveError(newUnmarshalTypeError(n, v.Type()))
		return nil
	}

	v = pv
	switch v.Kind() {
	case reflect.Interface:
		if t := v.Type(); t == nodeType || t == valueNodeType {
			v.Set(reflect.ValueOf(n))
			break
		}
		if v.NumMethod() != 0 {
			d.saveError(newUnmarshalTypeError(n, v.Type()))
			break
		}
		// Default to int if possible, otherwise float
		if n.IsInt {
			v.Set(reflect.ValueOf(int(n.Int64)))
		} else {
			v.Set(reflect.ValueOf(n.Float64))
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if !n.IsInt || v.OverflowInt(n.Int64) {
			d.saveError(newUnmarshalTypeError(n, v.Type()))
			break
		}
		v.SetInt(n.Int64)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if !n.IsUint || v.OverflowUint(n.Uint64) {
			d.saveError(newUnmarshalTypeError(n, v.Type()))
			break
		}
		v.SetUint(n.Uint64)

	case reflect.Float32, reflect.Float64:
		if !n.IsFloat || v.OverflowFloat(n.Float64) {
			d.saveError(newUnmarshalTypeError(n, v.Type()))
			break
		}
		v.SetFloat(n.Float64)

	case reflect.Struct:
		if v.Type() == reflect.TypeOf((*scparse.NumberNode)(nil)).Elem() {
			v.Set(reflect.ValueOf(n).Elem())
			break
		}
		fallthrough
	default:
		d.saveError(newUnmarshalTypeError(n, v.Type()))
	}
	return nil
}

func (d *decoder) decodeInterpolatedString(n *scparse.InterpolatedStringNode, v reflect.Value) error {
	// Check for unmarshaler.
	u, ut, pv := indirect(v, false)
	if u != nil {
		return u.UnmarshalSC(n, d.vars)
	}

	// Check for Node or ValueNode first so we don't unnecessarily expand variables
	if t := v.Type(); t == nodeType || t == valueNodeType {
		v.Set(reflect.ValueOf(n))
		return nil
	}

	// Hold off on checking TextUnmarshaler because we need to expand variables first
	var sb strings.Builder
	for _, c := range n.Components {
		switch c := c.(type) {
		case *scparse.StringNode:
			sb.WriteString(c.Value)
		case *scparse.VariableNode:
			// Lookup variable value
			val, ok := d.vars.Lookup(c)
			if !ok && d.disallowUnknownVars {
				d.saveError(&UnmarshalUnknownVariableError{Variable: c.Identifier.Name, Pos: c.Pos})
				return nil
			}
			if val == nil {
				break // coerce to empty string
			}
			sb.WriteString(fmt.Sprint(val))
		default:
			panic(fmt.Errorf("impossible: invalid node type in InterpolatedString: %T", c))
		}
	}
	if ut != nil {
		return ut.UnmarshalText([]byte(sb.String()))
	}

	v = pv
	switch v.Kind() {
	case reflect.Slice:
		// Handle []byte
		if v.Type().Elem().Kind() != reflect.Uint8 {
			d.saveError(newUnmarshalTypeError(n, v.Type()))
			break
		}
		src := []byte(sb.String())
		b := make([]byte, base64.StdEncoding.DecodedLen(len(src)))
		n, err := base64.StdEncoding.Decode(b, src)
		if err != nil {
			d.saveError(err)
			break
		}
		v.SetBytes(b[:n])
	case reflect.String:
		v.SetString(sb.String())
	case reflect.Interface:
		if v.NumMethod() == 0 {
			v.Set(reflect.ValueOf(sb.String()))
			break
		}
		d.saveError(newUnmarshalTypeError(n, v.Type()))
	case reflect.Struct:
		if v.Type() == reflect.TypeOf((*scparse.RawStringNode)(nil)).Elem() {
			v.Set(reflect.ValueOf(n).Elem())
			break
		}
		fallthrough
	default:
		d.saveError(newUnmarshalTypeError(n, v.Type()))
	}
	return nil
}

func (d *decoder) decodeRawString(n *scparse.RawStringNode, v reflect.Value) error {
	// Check for unmarshaler.
	u, ut, pv := indirect(v, false)
	if u != nil {
		return u.UnmarshalSC(n, d.vars)
	}
	if ut != nil {
		return ut.UnmarshalText([]byte(n.Value))
	}

	v = pv
	switch v.Kind() {
	case reflect.Slice:
		// Handle []byte
		if v.Type().Elem().Kind() != reflect.Uint8 {
			d.saveError(newUnmarshalTypeError(n, v.Type()))
			break
		}
		src := []byte(n.Value)
		b := make([]byte, base64.StdEncoding.DecodedLen(len(src)))
		n, err := base64.StdEncoding.Decode(b, src)
		if err != nil {
			d.saveError(err)
			break
		}
		v.SetBytes(b[:n])
	case reflect.String:
		v.SetString(n.Value)
	case reflect.Interface:
		if t := v.Type(); t == nodeType || t == valueNodeType {
			v.Set(reflect.ValueOf(n))
			break
		}
		if v.NumMethod() == 0 {
			v.Set(reflect.ValueOf(n.Value))
			break
		}
		d.saveError(newUnmarshalTypeError(n, v.Type()))
	case reflect.Struct:
		if v.Type() == reflect.TypeOf((*scparse.RawStringNode)(nil)).Elem() {
			v.Set(reflect.ValueOf(n).Elem())
			break
		}
		fallthrough
	default:
		d.saveError(newUnmarshalTypeError(n, v.Type()))
	}
	return nil
}

func (d *decoder) decodeVariable(n *scparse.VariableNode, v reflect.Value) error {
	// Check for unmarshaler.
	u, ut, pv := indirect(v, false)
	if u != nil {
		return u.UnmarshalSC(n, d.vars)
	}
	if ut != nil {
		d.saveError(newUnmarshalTypeError(n, v.Type()))
		return nil
	}

	t := v.Type()
	if t == nodeType || t == valueNodeType {
		v.Set(reflect.ValueOf(n))
		return nil
	}
	if t == reflect.TypeOf((*scparse.VariableNode)(nil)).Elem() {
		v.Set(reflect.ValueOf(n).Elem())
		return nil
	}

	// Lookup variable value
	val := d.vars.lookup(n)
	if !val.IsValid() {
		if d.disallowUnknownVars {
			d.saveError(&UnmarshalUnknownVariableError{Variable: n.Identifier.Name, Pos: n.Pos})
		}
		// Use the zero value of v
		return nil
	}
	// Unwrap interface
	if val.Kind() == reflect.Interface && !val.IsNil() {
		val = val.Elem()
	}

	v = pv
	switch valt := val.Type(); {
	case valt.AssignableTo(t):
		v.Set(val)
	case valt.ConvertibleTo(t):
		v.Set(val.Convert(t))
	default:
		d.saveError(newUnmarshalTypeError(n, t))
	}
	return nil
}

func (d *decoder) decodeDictionary(n *scparse.DictionaryNode, v reflect.Value) error {
	// Check for unmarshaler.
	u, ut, pv := indirect(v, false)
	if u != nil {
		return u.UnmarshalSC(n, d.vars)
	}
	if ut != nil {
		d.saveError(newUnmarshalTypeError(n, v.Type()))
		return nil
	}
	v = pv
	t := v.Type()

	// Decoding into nil interface? Switch to non-reflect code.
	if v.Kind() == reflect.Interface && v.NumMethod() == 0 {
		di := d.dictionaryInterface(n)
		v.Set(reflect.ValueOf(di))
		return nil
	}
	if t == nodeType || t == valueNodeType {
		v.Set(reflect.ValueOf(n))
		return nil
	}
	if t == reflect.TypeOf((*scparse.DictionaryNode)(nil)).Elem() {
		v.Set(reflect.ValueOf(n).Elem())
		return nil
	}

	var fields structFields

	// Check type of target:
	//   struct or map[T1]T2 where T1 is a string or an encoding.TextUnmarshaler
	switch v.Kind() {
	case reflect.Map:
		// Map key must either have a string kind or be an encoding.TextUnmarshaler.
		switch t.Key().Kind() {
		case reflect.String:
		default:
			if !reflect.PtrTo(t.Key()).Implements(textUnmarshalerType) {
				d.saveError(newUnmarshalTypeError(n, t))
				return nil
			}
		}
		if v.IsNil() {
			v.Set(reflect.MakeMap(t))
		}
	case reflect.Struct:
		fields = cachedTypeFields(t)
	default:
		d.saveError(newUnmarshalTypeError(n, t))
		return nil
	}

	var mapElem reflect.Value
	origErrorContext := d.errorContext

	for _, mn := range n.Members {
		key := mn.Key.KeyString()

		// Figure out field corresponding to key.
		var subv reflect.Value

		if v.Kind() == reflect.Map {
			elemType := t.Elem()
			if !mapElem.IsValid() {
				mapElem = reflect.New(elemType).Elem()
			} else {
				mapElem.Set(reflect.Zero(elemType))
			}
			subv = mapElem
		} else {
			var f *field
			if i, ok := fields.nameIndex[key]; ok {
				// Found an exact name match.
				f = &fields.list[i]
			} else {
				// Fall back to the expensive case-insensitive linear serach.
				for i := range fields.list {
					ff := &fields.list[i]
					if strings.EqualFold(ff.name, key) {
						f = ff
						break
					}
				}
			}
			if f != nil {
				subv = v
				for _, i := range f.index {
					if subv.Kind() == reflect.Ptr {
						if subv.IsNil() {
							// If a struct embeds a pointer to an unexported type,
							// it is not possible to set a newly allocated value
							// since the field is unexported.
							//
							// See https://golang.org/issue/21357
							if !subv.CanSet() {
								d.saveError(fmt.Errorf("sc: cannot set embedded pointer to unexported struct: %v", subv.Type().Elem()))
								// Invalidate subv to ensure d.decodeValue(subv) skips over
								// the SC value without assigning it to subv.
								subv = reflect.Value{}
								break
							}
							subv.Set(reflect.New(subv.Type().Elem()))
						}
						subv = subv.Elem()
					}
					subv = subv.Field(i)
				}
				d.errorContext.FieldStack = append(d.errorContext.FieldStack, f.name)
				d.errorContext.Struct = t
			} else if d.disallowUnknownFields {
				d.saveError(fmt.Errorf("sc: unknown field %q", key))
			}
			// ignore unknown field
		}

		if err := d.decodeValue(mn.Value, subv); err != nil {
			return err
		}

		// Write value back to map;
		// if using struct, subv points into struct already.
		if v.Kind() == reflect.Map {
			kt := t.Key()
			var kv reflect.Value
			switch {
			case reflect.PtrTo(kt).Implements(textUnmarshalerType):
				kv = reflect.New(kt)
				ut := kv.Interface().(encoding.TextUnmarshaler)
				if err := ut.UnmarshalText([]byte(key)); err != nil {
					return err
				}
				kv = kv.Elem()
			case kt.Kind() == reflect.String:
				kv = reflect.ValueOf(key).Convert(kt)
			default:
				panic("sc: Unexpected key type") // should never occur
			}
			if kv.IsValid() {
				v.SetMapIndex(kv, subv)
			}
		}

		// Reset errorContext to its original state.
		// Keep the same underlying array for FieldStack, to reuse the
		// space and avoid unnecessary allocs.
		d.errorContext.FieldStack = d.errorContext.FieldStack[:len(origErrorContext.FieldStack)]
		d.errorContext.Struct = origErrorContext.Struct
	}
	return nil
}

func (d *decoder) decodeList(n *scparse.ListNode, v reflect.Value) error {
	// Check for unmarshaler.
	u, ut, pv := indirect(v, false)
	if u != nil {
		return u.UnmarshalSC(n, d.vars)
	}
	if ut != nil {
		d.saveError(newUnmarshalTypeError(n, v.Type()))
		return nil
	}
	v = pv

	// Check type of target.
	switch v.Kind() {
	case reflect.Array, reflect.Slice:
		break
	case reflect.Interface:
		if t := v.Type(); t == nodeType || t == valueNodeType {
			v.Set(reflect.ValueOf(n))
			return nil
		}
		if v.NumMethod() == 0 {
			// Decoding into nil interface? Switch to non-reflect code.
			li := d.listInterface(n)
			v.Set(reflect.ValueOf(li))
			return nil
		}
		// Otherwise it's invalid.
		d.saveError(newUnmarshalTypeError(n, v.Type()))
	case reflect.Struct:
		if v.Type() == reflect.TypeOf((*scparse.ListNode)(nil)).Elem() {
			v.Set(reflect.ValueOf(n).Elem())
			return nil
		}
		fallthrough
	default:
		d.saveError(newUnmarshalTypeError(n, v.Type()))
		return nil
	}

	for i, e := range n.Elements {
		// Get element of list, growing the slice if necessary
		if v.Kind() == reflect.Slice {
			// Grow slice if necessary
			if i >= v.Cap() {
				newcap := v.Cap() + v.Cap()/2
				if newcap < 4 {
					newcap = 4
				}
				newv := reflect.MakeSlice(v.Type(), v.Len(), newcap)
				reflect.Copy(newv, v)
				v.Set(newv)
			}
			if i >= v.Len() {
				v.SetLen(i + 1)
			}
		}

		if i < v.Len() {
			// Decode into element.
			if err := d.decodeValue(e, v.Index(i)); err != nil {
				return err
			}
			continue
		}

		// Ran out of fixed array: skip.
		if err := d.decodeValue(e, reflect.Value{}); err != nil {
			return err
		}
	}

	count := len(n.Elements)
	if count < v.Len() {
		if v.Kind() == reflect.Array {
			// Array. Zero the rest.
			z := reflect.Zero(v.Type().Elem())
			for i := count; i < v.Len(); i++ {
				v.Index(i).Set(z)
			}
		} else {
			v.SetLen(count)
		}
	}
	// Handle empty slice
	if count == 0 && v.Kind() == reflect.Slice {
		v.Set(reflect.MakeSlice(v.Type(), 0, 0))
	}
	return nil
}

// The xxxInterface functions build up a value to be stored
// in an empty interface. They are not strictly necessary,
// but they avoid the weight of reflection in this common case.

// valueInterface is like decodeValue but returns interface{}
func (d *decoder) valueInterface(n scparse.ValueNode) interface{} {
	switch n := n.(type) {
	case *scparse.NullNode:
		return nil
	case *scparse.BoolNode:
		return n.True
	case *scparse.NumberNode:
		if n.IsInt {
			return int(n.Int64)
		}
		return n.Float64
	case *scparse.InterpolatedStringNode:
		var sb strings.Builder
		for _, c := range n.Components {
			switch c := c.(type) {
			case *scparse.StringNode:
				sb.WriteString(c.Value)
			case *scparse.VariableNode:
				// Lookup variable value
				val, ok := d.vars.Lookup(c)
				if !ok && d.disallowUnknownVars {
					d.saveError(&UnmarshalUnknownVariableError{Variable: c.Identifier.Name, Pos: c.Pos})
					return nil
				}
				if val == nil {
					break // coerce to empty string
				}
				sb.WriteString(fmt.Sprint(val))
			default:
				panic(fmt.Errorf("impossible: invalid node type in InterpolatedString: %T", c))
			}
		}
		return sb.String()
	case *scparse.RawStringNode:
		return n.Value
	case *scparse.VariableNode:
		val, ok := d.vars.Lookup(n)
		if !ok && d.disallowUnknownVars {
			d.saveError(&UnmarshalUnknownVariableError{Variable: n.Identifier.Name, Pos: n.Pos})
			return nil
		}
		return val
	case *scparse.DictionaryNode:
		return d.dictionaryInterface(n)
	case *scparse.ListNode:
		return d.listInterface(n)
	default:
		panic(fmt.Errorf("impossible: invalid node type used as value: %T", n))
	}
}

// dictionaryInterface is like decodeDictionary but returns map[string]interface{}
func (d *decoder) dictionaryInterface(n *scparse.DictionaryNode) map[string]interface{} {
	m := make(map[string]interface{})
	for _, mn := range n.Members {
		m[mn.Key.KeyString()] = d.valueInterface(mn.Value)
	}
	return m
}

// listInterface is like decodeList but returns []interface{}
func (d *decoder) listInterface(n *scparse.ListNode) []interface{} {
	v := make([]interface{}, len(n.Elements))
	for i, e := range n.Elements {
		v[i] = d.valueInterface(e)
	}
	return v
}

// indirect walks down v allocating pointers as needed, until it gets to a non-pointer.
// If it encounters an Unmarshaler or encoding.TextUnmarshaler, indirect stops and returns that.
// If decodingNull is true, indirect stops at the first settable pointer so it can be set to nil.
func indirect(v reflect.Value, decodingNull bool) (Unmarshaler, encoding.TextUnmarshaler, reflect.Value) {
	// Issue #24153 indicates that it is generally not a guaranteed property
	// that you may round-trip a reflect.Value by calling Value.Addr().Elem()
	// and expect the value to still be settable for values derived from
	// unexported embedded struct fields.
	//
	// The logic below effectively does this when it first addresses the value
	// (to satisfy possible pointer methods) and continues to dereference
	// subsequent pointers as necessary.
	//
	// After the first round-trip, we set v back to the original value to
	// preserve the original RW flags contained in reflect.Value.
	v0 := v
	haveAddr := false

	// If v is a named type and is addressable, start with its address,
	// so that if the type has pointer methods, we find them.
	if v.Kind() != reflect.Ptr && v.Type().Name() != "" && v.CanAddr() {
		haveAddr = true
		v = v.Addr()
	}
	for {
		// Load value from interface, but only if the result will be
		// usefully addressable.
		if v.Kind() == reflect.Interface && !v.IsNil() {
			e := v.Elem()
			if e.Kind() == reflect.Ptr && !e.IsNil() && (!decodingNull || e.Elem().Kind() == reflect.Ptr) {
				haveAddr = false
				v = e
				continue
			}
		}

		if v.Kind() != reflect.Ptr {
			break
		}

		if decodingNull && v.CanSet() {
			break
		}

		// Prevent infinite loop if v is an interface pointing to its own address:
		//     var v interface{}
		//     v = &v
		if v.Elem().Kind() == reflect.Interface && v.Elem().Elem() == v {
			v = v.Elem()
			break
		}
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		if v.Type().NumMethod() > 0 && v.CanInterface() {
			if u, ok := v.Interface().(Unmarshaler); ok {
				return u, nil, reflect.Value{}
			}
			if !decodingNull {
				if u, ok := v.Interface().(encoding.TextUnmarshaler); ok {
					return nil, u, reflect.Value{}
				}
			}
		}

		if haveAddr {
			v = v0 // restore original value after round-trip Value.Addr().Elem()
			haveAddr = false
		} else {
			v = v.Elem()
		}
	}
	return nil, nil, v
}
