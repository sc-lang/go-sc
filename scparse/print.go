// Copyright (c) 2021 the SC authors. All rights reserved. MIT License.

package scparse

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Format converts the SC AST to a textual representation and returns
// it as a byte slice.
//
// An effort will be made to preserve comments near the data they describe
// and format the data nicely. However, the original textual representation of
// the source is not guaranteed to be preserved.
func Format(n *DictionaryNode) []byte {
	p := &printer{}
	p.format(n)
	return p.Bytes()
}

// printer handles building the source string.
type printer struct {
	bytes.Buffer
	comments []Comment // pending end-of-line comments
	margin   int       // number of indents required
}

// printf prints to the buffer.
func (p *printer) printf(format string, args ...interface{}) {
	fmt.Fprintf(p, format, args...)
}

// indent prints the necessary indent.
func (p *printer) indent() {
	for i := 0; i < p.margin; i++ {
		p.WriteString("  ") // indent is 2 spaces
	}
}

// newline ends the current line, flushing end-of-line comments.
func (p *printer) newline() {
	if len(p.comments) > 0 {
		for _, c := range p.comments {
			p.WriteByte(' ')
			p.printComment(c)
		}
		p.comments = p.comments[:0]
	}
	p.WriteByte('\n')
	p.indent()
}

// format formats the SC document. This is the starting point for the printer.
func (p *printer) format(n *DictionaryNode) {
	p.printValue(n)
	p.newline()
	// Print trailing comments at the end of the document
	p.printComments(n.Comments().Foot)
}

func (p *printer) printComment(c Comment) {
	if c.IsBlock {
		p.printf("/*%s*/", c.Text)
		return
	}
	t := "//" + c.Text
	// Make sure only trailing space is trimmed, so do it after adding //
	p.WriteString(strings.TrimSpace(t))
}

func (p *printer) printComments(cmts []Comment) {
	// Assumption: multiple comments are separated by newlines
	for _, c := range cmts {
		p.printComment(c)
		p.newline()
	}
}

func (p *printer) printValue(n Node) {
	// Print comments before the node
	p.printComments(n.Comments().Head)

	switch n := n.(type) {
	// Simple cases, the string representation of these nodes can be used directly
	case *NullNode, *BoolNode, *RawStringNode, *VariableNode:
		p.WriteString(n.String())
	case *NumberNode:
		p.printNumber(n)
	case *InterpolatedStringNode:
		p.printInterpolatedString(n)
	case *ListNode:
		p.printList(n)
	case *DictionaryNode:
		p.printDictionary(n)
	default:
		panic(fmt.Errorf("printer: unexpected node type %T", n))
	}

	// Queue end-of-line comments
	p.comments = append(p.comments, n.Comments().Inline...)
}

func (p *printer) printNumber(n *NumberNode) {
	if n.Raw != "" {
		// Easy, we are done
		p.WriteString(n.Raw)
		return
	}
	// Harder, stringify the necessary number value
	if n.IsUint {
		p.printf("%d", n.Uint64)
		return
	}
	if n.IsInt {
		p.printf("%d", n.Int64)
		return
	}
	if n.IsFloat {
		p.printf("%f", n.Float64)
		return
	}
	// Empty number, print zero value
	p.WriteByte('0')
}

func (p *printer) printInterpolatedString(n *InterpolatedStringNode) {
	p.WriteByte('"')
	for _, c := range n.Components {
		switch c := c.(type) {
		case *StringNode:
			p.escapeString(c.Value)
		case *VariableNode:
			p.WriteString(c.String())
		default:
			panic(fmt.Errorf("printer: unexpected node type %T in InterpolatedStringNode", n))
		}
	}
	p.WriteByte('"')
}

func (p *printer) printList(n *ListNode) {
	p.WriteByte('[')

	// Handle empty list
	if len(n.Elements) == 0 {
		if inner := n.Comments().Inner; len(inner) > 0 {
			p.margin++
			for _, c := range inner {
				p.newline()
				p.printComment(c)
			}
			p.margin--
			p.newline()
		}
		p.WriteByte(']')
		return
	}

	p.margin++
	p.newline()
	// Inner comments will get converted into comments before the first element node
	p.printComments(n.Comments().Inner)
	for i, e := range n.Elements {
		p.printValue(e)
		for _, c := range e.Comments().Foot {
			p.newline()
			p.printComment(c)
		}
		if i < len(n.Elements)-1 {
			p.newline()
		}
	}
	p.margin--
	p.newline()
	p.WriteByte(']')
}

func (p *printer) printMember(n *MemberNode) {
	p.printComments(n.Comments().Head)

	k := n.Key
	p.printComments(k.Comments().Head)
	switch k := k.(type) {
	case *IdentifierNode, *RawStringNode:
		p.WriteString(k.String())
	case *StringNode:
		p.WriteByte('"')
		p.escapeString(k.Value)
		p.WriteByte('"')
	}

	p.comments = append(p.comments, k.Comments().Inline...)
	onOwnLine := false
	v := n.Value
	if len(p.comments) > 0 || len(k.Comments().Foot) > 0 || len(v.Comments().Head) > 0 {
		onOwnLine = true
		p.WriteByte(':')
		p.margin++
		p.newline()
	} else {
		p.WriteString(": ")
	}
	p.printComments(k.Comments().Foot)

	p.printValue(v)
	if onOwnLine {
		p.margin--
	}
}

func (p *printer) printDictionary(n *DictionaryNode) {
	p.WriteByte('{')

	// Handle empty dictionary
	if len(n.Members) == 0 {
		if inner := n.Comments().Inner; len(inner) > 0 {
			p.margin++
			for _, c := range inner {
				p.newline()
				p.printComment(c)
			}
			p.margin--
			p.newline()
		}
		p.WriteByte('}')
		return
	}

	p.margin++
	p.newline()
	// Inner comments will get converted into comments before the first member node
	p.printComments(n.Comments().Inner)
	for i, m := range n.Members {
		p.printMember(m)
		for _, c := range m.Comments().Foot {
			p.newline()
			p.printComment(c)
		}
		if i < len(n.Members)-1 {
			p.newline()
		}
	}
	p.margin--
	p.newline()
	p.WriteByte('}')
}

// escapeString converts the Go string to an SC string literal and
// writes it to the buffer.
func (p *printer) escapeString(s string) {
	start := 0
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if start < i {
				p.WriteString(s[start:i])
			}
			switch b {
			case '\\':
				p.WriteString(`\\`)
			case '"':
				p.WriteString(`\"`)
			case '\n':
				p.WriteString(`\n`)
			case '\r':
				p.WriteString(`\r`)
			case '\t':
				p.WriteString(`\t`)
			case '$':
				// $ is special because it only needs to be escaped if it's followed by a {
				if i < len(s)-1 && s[i+1] == '{' {
					p.WriteString(`\$`)
					break
				}
				fallthrough
			default:
				p.WriteByte(b)
			}
			i++
			start = i
			continue
		}
		c, size := utf8.DecodeRuneInString(s[i:])
		if c == utf8.RuneError && size == 1 {
			if start < i {
				p.WriteString(s[start:i])
			}
			p.WriteString(`\ufffd`)
			i += size
			start = i
			continue
		}
		i += size
	}
	if start < len(s) {
		p.WriteString(s[start:])
	}
}
