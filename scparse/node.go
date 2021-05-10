// Copyright (c) 2021 the SC authors. All rights reserved. MIT License.

package scparse

import (
	"fmt"
	"strconv"
	"strings"
)

// Pos represents a source position in the original input text.
type Pos struct {
	// Line is the line in the input text that this position occurs at.
	// Lines are counted starting at 1.
	Line int
	// Column is the column in the input text that this position occurs at.
	// Columns are counted starting at 1.
	//
	// Column takes unicode characters into account and reflects where the
	// character appears visually.
	Column int
	// Byte is the byte offset in the input where the position occurs.
	// Bytes are counted starting at 0.
	//
	// Byte reflects the exact byte in the input and does not take unicode
	// characters into account.
	Byte int
}

// Comment represents a comment. It can be either a line comment or a block comment.
type Comment struct {
	Pos     Pos    // Position of the comment in the input string.
	Text    string // The comment text with the // or /* stripped.
	IsBlock bool   // Is it a /*-style comment.
}

// CommentGroup contains all comments associated with a node.
type CommentGroup struct {
	Head   []Comment // Comments before the node.
	Inline []Comment // Comments after the node on the same line.

	// These are special cases and not generally used.

	// Comments after the node on a separate line.
	// Only used for comments after the last element of a list or dictionary.
	Foot  []Comment
	Inner []Comment // Comments inside an empty list or dictionary.
}

// NodeType identifies the type of an AST node.
type NodeType int

func (nt NodeType) String() string {
	return [...]string{
		"Null",
		"Bool",
		"Number",
		"String",
		"InterpolatedString",
		"RawString",
		"Identifier",
		"Variable",
		"List",
		"Member",
		"Dictionary",
		"end",
	}[nt]
}

const (
	NodeNull NodeType = iota
	NodeBool
	NodeNumber
	NodeString
	NodeInterpolatedString
	NodeRawString
	NodeIdentifier
	NodeVariable
	NodeList
	NodeMember
	NodeDictionary
	nodeEnd
)

// A Node is an element in the AST.
//
// The interface contains an unexported method so that only
// types in this package can implement it.
type Node interface {
	// Type identifies the type of the node.
	Type() NodeType
	// Position returns the position of the node in the input text.
	Position() Pos
	// Comments returns the comments attached to the node.
	Comments() *CommentGroup
	String() string
	// Helps with debugging in order to generate a textual
	// representation of a node in the AST.
	// This is private so this interface can only be satisfied
	// inside this package.
	writeTo(sb *strings.Builder)
}

// NullNode holds the special identifier 'null' representing the null value.
type NullNode struct {
	Pos          Pos
	CommentGroup CommentGroup
}

func (n *NullNode) Type() NodeType {
	return NodeNull
}

func (n *NullNode) Position() Pos {
	return n.Pos
}

func (n *NullNode) Comments() *CommentGroup {
	return &n.CommentGroup
}

func (n *NullNode) String() string {
	return "null"
}

func (n *NullNode) writeTo(sb *strings.Builder) {
	sb.WriteString(n.String())
}

// BoolNode holds a boolean value.
type BoolNode struct {
	Pos          Pos
	CommentGroup CommentGroup
	True         bool // The boolean value.
}

func (n *BoolNode) Type() NodeType {
	return NodeBool
}

func (n *BoolNode) Position() Pos {
	return n.Pos
}

func (n *BoolNode) Comments() *CommentGroup {
	return &n.CommentGroup
}

func (n *BoolNode) String() string {
	if n.True {
		return "true"
	}
	return "false"
}

func (n *BoolNode) writeTo(sb *strings.Builder) {
	sb.WriteString(n.String())
}

// NumberNode holds a number, either an int or a float.
// The value is parsed and stored under all types that can represent
// the value.
type NumberNode struct {
	Pos          Pos
	CommentGroup CommentGroup
	IsUint       bool    // The number has a unsigned int value.
	IsInt        bool    // The number has an int value.
	IsFloat      bool    // The number has a float value.
	Uint64       uint64  // The unsigned int value.
	Int64        int64   // The int value.
	Float64      float64 // The float value.
	Raw          string  // The raw string value from the input.
}

// newNumber creates a new number node by parsing raw.
func newNumber(pos Pos, raw string) (*NumberNode, error) {
	n := &NumberNode{Pos: pos, Raw: raw}

	// Check if it is an int first
	u, err := strconv.ParseUint(raw, 10, 64)
	if err == nil {
		n.IsUint = true
		n.Uint64 = u
	}
	i, err := strconv.ParseInt(raw, 10, 64)
	if err == nil {
		n.IsInt = true
		n.Int64 = i
		if i == 0 {
			// ParseUint fails for -0, fix it here
			n.IsUint = true
			n.Uint64 = 0
		}
	}

	// If number is an int, then it's automatically a float
	if n.IsInt {
		n.IsFloat = true
		n.Float64 = float64(n.Int64)
	} else {
		f, err := strconv.ParseFloat(raw, 64)
		if err == nil {
			// If it looks like an int, it's too large of a number
			if !strings.ContainsAny(raw, ".eE") {
				return nil, fmt.Errorf("integer overflow: %q", raw)
			}
			n.IsFloat = true
			n.Float64 = f

			// See if the float is a valid int
			// This can happen if there is an exponent present
			// Ex: 1e4 == 10000 which is a valid int
			if !n.IsInt && float64(int64(f)) == f {
				n.IsInt = true
				n.Int64 = int64(f)
			}
			if !n.IsUint && float64(uint64(f)) == f {
				n.IsUint = true
				n.Uint64 = uint64(f)
			}
		}
	}
	// Make sure at least one type of number was parsed
	if !n.IsUint && !n.IsInt && !n.IsFloat {
		return nil, fmt.Errorf("invalid number syntax: %q", raw)
	}
	return n, nil
}

func (n *NumberNode) Type() NodeType {
	return NodeNumber
}

func (n *NumberNode) Position() Pos {
	return n.Pos
}

func (n *NumberNode) Comments() *CommentGroup {
	return &n.CommentGroup
}

func (n *NumberNode) String() string {
	return n.Raw
}

func (n *NumberNode) writeTo(sb *strings.Builder) {
	sb.WriteString(n.String())
}

// StringNode holds a string value. The quotes have been removed.
type StringNode struct {
	Pos          Pos
	CommentGroup CommentGroup
	Value        string // The string value, after quotes have been removed.
}

func (n *StringNode) Type() NodeType {
	return NodeString
}

func (n *StringNode) Position() Pos {
	return n.Pos
}

func (n *StringNode) Comments() *CommentGroup {
	return &n.CommentGroup
}

func (n *StringNode) String() string {
	return `"` + n.Value + `"`
}

func (n *StringNode) writeTo(sb *strings.Builder) {
	sb.WriteString(n.String())
}

// Key implements the KeyNode interface by returning the string value.
func (n *StringNode) Key() string {
	return n.Value
}

// InterpolatedStringNode is a double quoted string that may have
// variables interpolated into it. It contains a list of component nodes
// which are either StringNodes or VariableNodes.
type InterpolatedStringNode struct {
	Pos          Pos
	CommentGroup CommentGroup
	Components   []Node // Each component is either a StringNode or VariableNode.
}

func (n *InterpolatedStringNode) Type() NodeType {
	return NodeInterpolatedString
}

func (n *InterpolatedStringNode) Position() Pos {
	return n.Pos
}

func (n *InterpolatedStringNode) Comments() *CommentGroup {
	return &n.CommentGroup
}

func (n *InterpolatedStringNode) String() string {
	var sb strings.Builder
	n.writeTo(&sb)
	return sb.String()
}

func (n *InterpolatedStringNode) writeTo(sb *strings.Builder) {
	sb.WriteByte('"')
	for _, c := range n.Components {
		if c.Type() == NodeVariable {
			c.writeTo(sb)
		} else if c.Type() == NodeString {
			sb.WriteString(c.(*StringNode).Value)
		} else {
			panic(fmt.Errorf("invalid node type %T in InterpolatedStringNode", c))
		}
	}
	sb.WriteByte('"')
}

// RawStringNode holds a raw string value. The quotes have been removed.
type RawStringNode struct {
	Pos          Pos
	CommentGroup CommentGroup
	Value        string // The string value, after quotes have been removed.
}

func (n *RawStringNode) Type() NodeType {
	return NodeRawString
}

func (n *RawStringNode) Position() Pos {
	return n.Pos
}

func (n *RawStringNode) Comments() *CommentGroup {
	return &n.CommentGroup
}

func (n *RawStringNode) String() string {
	return "`" + n.Value + "`"
}

func (n *RawStringNode) writeTo(sb *strings.Builder) {
	sb.WriteString(n.String())
}

// Key implements the KeyNode interface by returning the string value.
func (n *RawStringNode) Key() string {
	return n.Value
}

// IdentifierNode holds an identifier.
type IdentifierNode struct {
	Pos          Pos
	CommentGroup CommentGroup
	Name         string // The identifier name.
}

func (n *IdentifierNode) Type() NodeType {
	return NodeIdentifier
}

func (n *IdentifierNode) Position() Pos {
	return n.Pos
}

func (n *IdentifierNode) Comments() *CommentGroup {
	return &n.CommentGroup
}

func (n *IdentifierNode) String() string {
	return n.Name
}

func (n *IdentifierNode) writeTo(sb *strings.Builder) {
	sb.WriteString(n.String())
}

// Key implements the KeyNode interface by returning the identifier value.
func (n *IdentifierNode) Key() string {
	return n.Name
}

// VariableNode holds a variable.
type VariableNode struct {
	Pos          Pos
	CommentGroup CommentGroup
	Identifier   *IdentifierNode // The variable name.
}

func (n *VariableNode) Type() NodeType {
	return NodeVariable
}

func (n *VariableNode) Position() Pos {
	return n.Pos
}

func (n *VariableNode) Comments() *CommentGroup {
	return &n.CommentGroup
}

func (n *VariableNode) String() string {
	var sb strings.Builder
	n.writeTo(&sb)
	return sb.String()
}

func (n *VariableNode) writeTo(sb *strings.Builder) {
	sb.WriteString("${")
	n.Identifier.writeTo(sb)
	sb.WriteByte('}')
}

// ListNode holds a list that contains a sequence of nodes.
type ListNode struct {
	Pos          Pos
	CommentGroup CommentGroup
	Elements     []Node // The elements in the order they were scanned.
}

func (n *ListNode) Type() NodeType {
	return NodeList
}

func (n *ListNode) Position() Pos {
	return n.Pos
}

func (n *ListNode) Comments() *CommentGroup {
	return &n.CommentGroup
}

func (n *ListNode) String() string {
	var sb strings.Builder
	n.writeTo(&sb)
	return sb.String()
}

func (n *ListNode) writeTo(sb *strings.Builder) {
	sb.WriteByte('[')
	for i, el := range n.Elements {
		if i > 0 {
			sb.WriteString(", ")
		}
		el.writeTo(sb)
	}
	sb.WriteByte(']')
}

// KeyNode is a special type of Node that can act as a dictionary key.
type KeyNode interface {
	Node
	Key() string
}

// MemberNode holds a member of a dictionary.
// It contains the key and the value.
type MemberNode struct {
	Pos          Pos
	CommentGroup CommentGroup
	Key          KeyNode // The key of the member.
	Value        Node    // The value of the member.
}

func (n *MemberNode) Type() NodeType {
	return NodeMember
}

func (n *MemberNode) Position() Pos {
	return n.Pos
}

func (n *MemberNode) Comments() *CommentGroup {
	return &n.CommentGroup
}

func (n *MemberNode) String() string {
	var sb strings.Builder
	n.writeTo(&sb)
	return sb.String()
}

func (n *MemberNode) writeTo(sb *strings.Builder) {
	n.Key.writeTo(sb)
	sb.WriteString(": ")
	n.Value.writeTo(sb)
}

// DictionaryNode holds a dictionary that contains a list of members.
type DictionaryNode struct {
	Pos          Pos
	CommentGroup CommentGroup
	Members      []*MemberNode // The members in the order they were scanned.
}

func (n *DictionaryNode) Type() NodeType {
	return NodeDictionary
}

func (n *DictionaryNode) Position() Pos {
	return n.Pos
}

func (n *DictionaryNode) Comments() *CommentGroup {
	return &n.CommentGroup
}

func (n *DictionaryNode) String() string {
	var sb strings.Builder
	n.writeTo(&sb)
	return sb.String()
}

func (n *DictionaryNode) writeTo(sb *strings.Builder) {
	sb.WriteByte('{')
	for i, m := range n.Members {
		if i > 0 {
			sb.WriteString(", ")
		}
		m.writeTo(sb)
	}
	sb.WriteByte('}')
}

// endNode represents the end of a list or dictionary.
// It only exists to aid parsing, it is not added to the AST.
type endNode struct {
	Pos          Pos
	CommentGroup CommentGroup
}

func (n *endNode) Type() NodeType {
	return nodeEnd
}

func (n *endNode) Position() Pos {
	return n.Pos
}

func (n *endNode) Comments() *CommentGroup {
	return &n.CommentGroup
}

func (n *endNode) String() string {
	return ""
}

func (n *endNode) writeTo(sb *strings.Builder) {}
