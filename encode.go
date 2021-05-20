// Copyright (c) 2021 the SC authors. All rights reserved. MIT License.

package sc

import (
	"encoding"
	"encoding/base64"
	"fmt"
	"reflect"
	"sort"
	"unicode"

	"github.com/sc-lang/go-sc/scparse"
)

var (
	marshalerType     = reflect.TypeOf((*Marshaler)(nil)).Elem()
	textMarshalerType = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
)

// scError is an error wrapper to distinguish intentional panics.
type scError struct{ error }

// encoder encodes Go values into SC nodes.
type encoder struct{}

// error terminates encoding by panicking with err.
func (e *encoder) error(err error) {
	panic(scError{err})
}

// marshalErrorf creates a MarshalError and calls error with it.
func (e *encoder) marshalErrorf(v reflect.Value, format string, args ...interface{}) {
	e.error(&MarshalError{Value: v, Context: fmt.Sprintf(format, args...)})
}

func (e *encoder) marshal(v interface{}) (n *scparse.DictionaryNode, err error) {
	defer func() {
		if r := recover(); r != nil {
			if serr, ok := r.(scError); ok {
				err = serr.error
				return
			}
			panic(r)
		}
	}()

	vv := reflect.ValueOf(v)
	vn := e.encodeValue(vv)
	var ok bool
	n, ok = vn.(*scparse.DictionaryNode)
	if !ok {
		e.marshalErrorf(vv, "unsupported type: %s", vv.Type())
	}
	return n, nil
}

func (e *encoder) encodeValue(v reflect.Value) scparse.ValueNode {
	t := v.Type()
	if t.Implements(nodeType) {
		return e.encodeNode(v)
	}
	// Check if it is node that isn't a pointer. This is supported for consistency since
	// Unmarshal supports non-pointer nodes as a consequence of how it is implemented.
	if reflect.PtrTo(t).Implements(nodeType) {
		if !v.CanAddr() {
			nv := reflect.New(t).Elem()
			nv.Set(v)
			v = nv
		}
		return e.encodeNode(v.Addr())
	}
	if t.Implements(marshalerType) {
		return e.encodeMarshaler(v)
	}
	if t.Implements(textMarshalerType) {
		return e.encodeTextMarshaler(v)
	}

	switch v.Kind() {
	case reflect.Bool:
		return &scparse.BoolNode{True: v.Bool()}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return &scparse.NumberNode{IsUint: true, Uint64: v.Uint()}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &scparse.NumberNode{IsInt: true, Int64: v.Int()}
	case reflect.Float32, reflect.Float64:
		return &scparse.NumberNode{IsFloat: true, Float64: v.Float()}
	case reflect.String:
		return newDoubleString(v.String())
	case reflect.Interface, reflect.Ptr:
		return e.encodeInterfaceOrPtr(v)
	case reflect.Array, reflect.Slice:
		return e.encodeArrayOrSlice(v)
	case reflect.Map:
		return e.encodeMap(v)
	case reflect.Struct:
		return e.encodeStruct(v)
	default:
		e.marshalErrorf(v, "unsupported type: %s", t)
	}
	// Never hit, go doesn't realize that the default cause panics though
	return nil
}

func (e *encoder) encodeNode(v reflect.Value) scparse.ValueNode {
	// We know v implements Node, need to narrow it down and handle non-value nodes.
	// Safe because v must be an interface or pointer (since all node implementations
	// have pointer receivers).
	if v.IsNil() {
		return &scparse.NullNode{}
	}
	if v.Kind() == reflect.Interface {
		return e.encodeNode(v.Elem())
	}
	n, ok := v.Interface().(scparse.ValueNode)
	if !ok {
		e.marshalErrorf(v, "unsupported node type %s; not a value node", v.Type())
	}
	return n
}

func (e *encoder) encodeMarshaler(v reflect.Value) scparse.ValueNode {
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return &scparse.NullNode{}
	}
	m, ok := v.Interface().(Marshaler)
	if !ok {
		return &scparse.NullNode{}
	}
	n, err := m.MarshalSC()
	if err != nil {
		e.error(err)
	}
	return n
}

func (e *encoder) encodeTextMarshaler(v reflect.Value) scparse.ValueNode {
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return &scparse.NullNode{}
	}
	m, ok := v.Interface().(encoding.TextMarshaler)
	if !ok {
		return &scparse.NullNode{}
	}
	b, err := m.MarshalText()
	if err != nil {
		e.error(err)
	}
	return newDoubleString(string(b))
}

func (e *encoder) encodeInterfaceOrPtr(v reflect.Value) scparse.ValueNode {
	if v.IsNil() {
		return &scparse.NullNode{}
	}
	return e.encodeValue(v.Elem())
}

func (e *encoder) encodeArrayOrSlice(v reflect.Value) scparse.ValueNode {
	if v.Kind() == reflect.Slice {
		if v.IsNil() {
			return &scparse.NullNode{}
		}
		// []byte is encoded as a base64 string
		if t := v.Type(); t.Elem().Kind() == reflect.Uint8 {
			p := reflect.PtrTo(t.Elem())
			if !p.Implements(marshalerType) && !p.Implements(textMarshalerType) {
				return newDoubleString(base64.StdEncoding.EncodeToString(v.Bytes()))
			}
		}
	}
	vlen := v.Len()
	elements := make([]scparse.ValueNode, vlen)
	for i := 0; i < vlen; i++ {
		elements[i] = e.encodeValue(v.Index(i))
	}
	return &scparse.ListNode{Elements: elements}
}

func (e *encoder) encodeMap(v reflect.Value) scparse.ValueNode {
	// Check that the map key is a valid type
	if kt := v.Type().Key(); kt.Kind() != reflect.String && !kt.Implements(textMarshalerType) {
		e.marshalErrorf(v, "unsupported map with key type: %s", kt)
	}
	if v.IsNil() {
		return &scparse.NullNode{}
	}

	// Get the keys.
	keys := v.MapKeys()
	mapKeys := make([]mapKey, len(keys))
	for i, k := range keys {
		mk, err := resolveMapKey(k)
		if err != nil {
			e.error(fmt.Errorf("sc: error encoding type %s: %w", v.Type(), err))
		}
		mapKeys[i] = mk
	}
	// Sort the keys. This ensures that encoding is deterministic.
	sort.Slice(mapKeys, func(i, j int) bool {
		return mapKeys[i].s < mapKeys[j].s
	})

	members := make([]*scparse.MemberNode, len(mapKeys))
	for i, mk := range mapKeys {
		vn := e.encodeValue(v.MapIndex(mk.v))
		members[i] = &scparse.MemberNode{Key: e.encodeKey(mk.s), Value: vn}
	}
	return &scparse.DictionaryNode{Members: members}
}

func (e *encoder) encodeStruct(v reflect.Value) scparse.ValueNode {
	fields := cachedTypeFields(v.Type())
	members := make([]*scparse.MemberNode, 0, len(fields.list))
Loop:
	for i := range fields.list {
		f := &fields.list[i]
		fv := v
		for _, j := range f.index {
			if fv.Kind() == reflect.Ptr {
				if fv.IsNil() {
					continue Loop
				}
				fv = fv.Elem()
			}
			fv = fv.Field(j)
		}
		if f.omitEmpty && isEmpty(fv) {
			continue
		}
		m := &scparse.MemberNode{Key: e.encodeKey(f.name), Value: e.encodeValue(fv)}
		members = append(members, m)
	}
	return &scparse.DictionaryNode{Members: members}
}

func (e *encoder) encodeKey(s string) scparse.KeyNode {
	needsQuote := false
	for i, r := range s {
		if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
			needsQuote = true
			break
		}
	}
	if needsQuote {
		return &scparse.StringNode{Value: s}
	}
	return &scparse.IdentifierNode{Name: s}
}

type mapKey struct {
	v reflect.Value
	s string
}

// resolveMapKey resolves the string value of a map key.
func resolveMapKey(v reflect.Value) (mapKey, error) {
	if v.Kind() == reflect.String {
		return mapKey{v, v.String()}, nil
	}
	m, ok := v.Interface().(encoding.TextMarshaler)
	if !ok {
		panic(fmt.Errorf("impossible: unexpected map key type: %s", v.Type()))
	}
	b, err := m.MarshalText()
	return mapKey{v, string(b)}, err
}

func isEmpty(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	}
	return false
}

// newDoubleString is a small convenience function to return an
// InterpolatedStringNode from a single string value.
func newDoubleString(s string) *scparse.InterpolatedStringNode {
	sn := &scparse.StringNode{Value: s}
	return &scparse.InterpolatedStringNode{Components: []scparse.StringContentNode{sn}}
}
