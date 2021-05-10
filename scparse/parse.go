// Copyright (c) 2021 the SC authors. All rights reserved. MIT License.

// Package scparse implements parsing and formatting of SC files.
// It provides node types that are used to build an abstract syntax tree (AST).
//
// The Parse function parses SC source text and creates an AST.
//
// The Format function formats an AST back to source text.
//
// In general, clients should use the sc package for working with SC data
// in Go. This package should only be used if you need to directly maniplate
// AST nodes. An example use case would be to access comments in an SC file.
package scparse

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// Error represents an error that occurred during parsing.
// It contains information about the context of the error.
type Error struct {
	// Pos is the position of the error in the source.
	Pos Pos
	// Context contains the details of the error.
	Context string
}

func (e *Error) Error() string {
	return fmt.Sprintf("sc: Parse Error: %d:%d: %s", e.Pos.Line, e.Pos.Column, e.Context)
}

// Parse parses the SC source and generates an AST.
// If err is not nil, it will contain details on the error
// encountered and it's location in input.
func Parse(input []byte) (n *DictionaryNode, err error) {
	p := &parser{lex: lex(input)}
	defer p.recover(&err)
	n = p.parse()
	// Dispose of the lexer since we don't need it anymore
	p.lex = nil
	return n, nil
}

// parser handles parsing a SC document into an AST.
type parser struct {
	lex       *lexer
	token     token // one token lookahead
	hasPeeked bool
}

// next returns the next token.
func (p *parser) next() token {
	if p.hasPeeked {
		p.hasPeeked = false
	} else {
		p.token = p.lex.nextToken()
	}
	return p.token
}

// peek returns but does not consume the next token.
func (p *parser) peek() token {
	if p.hasPeeked {
		return p.token
	}
	p.hasPeeked = true
	p.token = p.lex.nextToken()
	return p.token
}

// errorf formats the error and terminates processing.
func (p *parser) errorf(format string, args ...interface{}) {
	panic(&Error{Pos: p.token.pos, Context: fmt.Sprintf(format, args...)})
}

// unexpected complains about the token and terminates processing.
func (p *parser) unexpected(tok token, context string) {
	if tok.typ == tokenError {
		p.errorf("%s", tok)
	}
	p.errorf("unexpected %s in %s", tok, context)
}

// expect consumes the next token and guarantees it has the required type.
func (p *parser) expect(expected tokenType, context string) token {
	tok := p.next()
	if tok.typ != expected {
		p.unexpected(tok, context)
	}
	return tok
}

// recover turns panics into returns from the top level of Parse.
func (p *parser) recover(errp *error) {
	e := recover()
	if e == nil {
		return
	}
	// Make sure it's an Error otherwise it is something more serious that we can't handle
	// (ex: runtime.Error)
	if _, ok := e.(*Error); !ok {
		panic(e)
	}
	if p.lex != nil {
		p.lex.drain()
	}
	*errp = e.(error)
}

// parse is the top level parser that parses the SC document.
func (p *parser) parse() *DictionaryNode {
	n := p.parseValue()
	if n.Type() != NodeDictionary {
		// Overwrite the pos of the token so that the error is reported
		// at the start of the node, not at the end
		p.token.pos = n.Position()
		p.errorf("top level value in SC document must be a dictionary")
	}

	// Handle any remaining tokens. At this point all we can have are comments,
	// a single trailing comma, and EOF.
	var commaTok token
	seenComma := false
Loop:
	for {
		switch p.peek().typ {
		case tokenEOF:
			p.next()
			break Loop
		case tokenComment:
			c := n.Comments()
			// Check if comment is actually inline with the comma
			if seenComma && commaTok.pos.Line == p.peek().pos.Line {
				c.Inline = append(c.Inline, p.parseComment())
				break
			}
			c.Foot = append(c.Foot, p.parseComment())
		case tokenComma:
			if !seenComma {
				commaTok = p.next()
				seenComma = true
				break
			}
			fallthrough
		default:
			p.unexpected(p.next(), "end of document")
		}
	}
	return n.(*DictionaryNode)
}

// parseValue parses a SC value and returns the appropriate node.
func (p *parser) parseValue() Node {
	// Parse head comments before the node
	var headComments []Comment
	for p.peek().typ == tokenComment {
		headComments = append(headComments, p.parseComment())
	}

	var node Node
	switch p.peek().typ {
	case tokenNull:
		node = p.parseNull()
	case tokenBool:
		node = p.parseBool()
	case tokenNumber:
		node = p.parseNumber()
	case tokenQuote:
		node = p.parseString()
	case tokenRawString:
		node = p.parseRawString()
	case tokenVariableStart:
		node = p.parseVariable()
	case tokenLeftCurlyParen:
		node = p.parseDictionary()
	case tokenLeftSquareParen:
		node = p.parseList()
	case tokenRightSquareParen:
		// Handle end of list
		node = &endNode{Pos: p.next().pos}
	default:
		p.unexpected(p.next(), "value")
	}

	c := node.Comments()
	c.Head = append(c.Head, headComments...)
	// Parse inline comments right after the node
	c.Inline = append(c.Inline, p.parseInlineComments(node.Position().Line)...)
	return node
}

// parseComment parses either a // or /* comment
func (p *parser) parseComment() Comment {
	tok := p.next()
	var text string
	isBlock := false
	if strings.HasPrefix(tok.val, "//") {
		text = strings.TrimPrefix(tok.val, "//")
	} else {
		text = strings.TrimPrefix(tok.val, "/*")
		text = strings.TrimSuffix(text, "*/")
		isBlock = true
	}
	return Comment{Pos: tok.pos, Text: text, IsBlock: isBlock}
}

// parseInlineComments parses all comments that are on line.
func (p *parser) parseInlineComments(line int) []Comment {
	var comments []Comment
	for {
		tok := p.peek()
		if tok.typ != tokenComment || tok.pos.Line != line {
			break
		}
		comments = append(comments, p.parseComment())
	}
	return comments
}

func (p *parser) parseNull() *NullNode {
	tok := p.next()
	return &NullNode{Pos: tok.pos}
}

func (p *parser) parseBool() *BoolNode {
	tok := p.next()
	// We have a 50% chance of being right :)
	val := false
	if tok.val == "true" {
		val = true
	}
	return &BoolNode{Pos: tok.pos, True: val}
}

func (p *parser) parseNumber() *NumberNode {
	tok := p.next()
	n, err := newNumber(tok.pos, tok.val)
	if err != nil {
		p.errorf("%s", err)
	}
	return n
}

// parseStringKey parses string values meant for dictionary keys.
// It does not allow interpolated strings.
func (p *parser) parseStringKey() *StringNode {
	startTok := p.next()
	var sb strings.Builder
Loop:
	for {
		switch tok := p.next(); tok.typ {
		case tokenQuote:
			break Loop
		// Right curly paren is a false positive by the lexer
		case tokenString, tokenRightCurlyParen:
			sb.WriteString(p.unescapeString(tok.val))
		case tokenVariableStart:
			// Handle variable explicitly so we can give a good error message
			p.unexpected(tok, "string key, dictionary keys cannot contain variables")
		default:
			p.unexpected(tok, "string key")
		}
	}
	return &StringNode{Pos: startTok.pos, Value: sb.String()}
}

// parseString parses a string value that might have variables interpolated in it.
func (p *parser) parseString() *InterpolatedStringNode {
	startTok := p.next()
	var components []Node
	// To combine and normalize false positives into a single string
	var sn *StringNode
	var sb strings.Builder
Loop:
	for {
		switch p.peek().typ {
		case tokenQuote:
			p.next()
			break Loop
		// Right curly paren is a false positive by the lexer
		case tokenString, tokenRightCurlyParen:
			tok := p.next()
			if sn == nil {
				sn = &StringNode{Pos: tok.pos}
			}
			sb.WriteString(p.unescapeString(tok.val))
		case tokenVariableStart:
			if sn != nil {
				sn.Value = sb.String()
				components = append(components, sn)
				sn = nil
				sb.Reset()
			}
			components = append(components, p.parseVariable())
		default:
			p.unexpected(p.next(), "string value")
		}
	}
	if sn != nil {
		sn.Value = sb.String()
		components = append(components, sn)
	}
	return &InterpolatedStringNode{Pos: startTok.pos, Components: components}
}

func (p *parser) parseRawString() *RawStringNode {
	tok := p.next()
	// Strip quotes
	s := tok.val[1 : len(tok.val)-1]
	return &RawStringNode{Pos: tok.pos, Value: s}
}

func (p *parser) parseVariable() *VariableNode {
	// Variable start, i.e. ${
	startTok := p.next()
	idTok := p.expect(tokenIdentifier, "variable")
	p.expect(tokenRightCurlyParen, "variable, expected '}'")
	id := &IdentifierNode{Pos: idTok.pos, Name: idTok.val}
	return &VariableNode{Pos: startTok.pos, Identifier: id}
}

func (p *parser) parseMember() Node {
	// Parse head comments before the node
	var headComments []Comment
	for p.peek().typ == tokenComment {
		headComments = append(headComments, p.parseComment())
	}

	// Handle end of list
	if p.peek().typ == tokenRightCurlyParen {
		end := &endNode{Pos: p.next().pos}
		end.Comments().Head = headComments
		end.Comments().Inline = p.parseInlineComments(end.Position().Line)
		return end
	}

	// First parse the key,  it must be an identifier or a string
	var key KeyNode
	switch p.peek().typ {
	case tokenIdentifier:
		tok := p.next()
		key = &IdentifierNode{Pos: tok.pos, Name: tok.val}
	case tokenQuote:
		key = p.parseStringKey()
	case tokenRawString:
		key = p.parseRawString()
	default:
		p.unexpected(p.next(), "dictionary key, expected identifier or string")
	}

	key.Comments().Head = headComments
	key.Comments().Inline = p.parseInlineComments(key.Position().Line)
	// Next token must be a colon
	p.expect(tokenColon, "dictionary element, expected ':'")

	// Value can be any value, and so we recurse
	val := p.parseValue()
	return &MemberNode{Pos: key.Position(), Key: key, Value: val}
}

func (p *parser) parseDictionary() *DictionaryNode {
	startTok := p.next()
	var members []*MemberNode
	var end Node
	for {
		mem := p.parseMember()
		// Handle end of dictionary
		if mem.Type() == nodeEnd {
			end = mem
			break
		}
		memNode := mem.(*MemberNode)
		members = append(members, memNode)
		// Next token must either be comma or end of dictionary
		if p.peek().typ == tokenRightCurlyParen {
			// Have parseMember handle end of dictionary so it also parses comments
			continue
		}
		tok := p.expect(tokenComma, "dictionary, expected ','")
		// Might be additional inline comments after the comma
		c := memNode.Value.Comments()
		// Use the comma pos not the element pos because some elements
		// might span multiple lines
		c.Inline = append(c.Inline, p.parseInlineComments(tok.pos.Line)...)
	}

	dict := &DictionaryNode{Pos: startTok.pos, Members: members}
	dict.Comments().Inline = end.Comments().Inline
	if len(members) == 0 {
		dict.Comments().Inner = end.Comments().Head
	} else {
		lastMem := members[len(members)-1]
		lastMem.Comments().Foot = end.Comments().Head
	}
	return dict
}

func (p *parser) parseList() *ListNode {
	startTok := p.next()
	var elements []Node
	var end Node
	for {
		el := p.parseValue()
		// Handle end of list
		if el.Type() == nodeEnd {
			end = el
			break
		}

		elements = append(elements, el)
		// Next token must either be comma or end of list
		if p.peek().typ == tokenRightSquareParen {
			// Have parseValue handle end of list so it also parses comments
			continue
		}
		tok := p.expect(tokenComma, "list, expected ','")
		// Might be additional inline comments after the comma
		c := el.Comments()
		// Use the comma pos not the element pos because some elements
		// might span multiple lines
		c.Inline = append(c.Inline, p.parseInlineComments(tok.pos.Line)...)
	}

	list := &ListNode{Pos: startTok.pos, Elements: elements}
	// Handle comments on endNode
	list.Comments().Inline = end.Comments().Inline
	if len(elements) == 0 {
		list.Comments().Inner = end.Comments().Head
	} else {
		lastEl := elements[len(elements)-1]
		lastEl.Comments().Foot = end.Comments().Head
	}
	return list
}

// The unescapeString and getu4 functions were adapted from encoding/json.
// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// unescapeString converts a quoted SC string literal into an actual string.
// It will replace escape characters with their actual values.
// str should not be quoted.
func (p *parser) unescapeString(str string) string {
	s := []byte(str)
	// Check for unusual characters. If there are none,
	// then no unescaping is needed, so return the original string.
	r := 0
	for r < len(s) {
		c := s[r]
		if c == '\\' || c == '"' || c < ' ' {
			break
		}
		if c < utf8.RuneSelf {
			r++
			continue
		}
		rr, size := utf8.DecodeRune(s[r:])
		if rr == utf8.RuneError && size == 1 {
			break
		}
		r += size
	}
	if r == len(s) {
		return str
	}

	b := make([]byte, len(s)+2*utf8.UTFMax)
	w := copy(b, s[0:r])
	for r < len(s) {
		// Out of room? Can only happen if s is full of
		// malformed UTF-8 and we're replacing each
		// byte with RuneError.
		if w >= len(b)-2*utf8.UTFMax {
			nb := make([]byte, (len(b)+utf8.UTFMax)*2)
			copy(nb, b[0:w])
			b = nb
		}
		switch c := s[r]; {
		case c == '\\':
			r++
			if r >= len(s) {
				p.errorf("unterminated escape character")
			}
			switch s[r] {
			default:
				p.errorf(`invalid escape character '\%c' in string`, s[r])
			case '"', '\\', '/', '\'', '$':
				b[w] = s[r]
				r++
				w++
			case 'b':
				b[w] = '\b'
				r++
				w++
			case 'f':
				b[w] = '\f'
				r++
				w++
			case 'n':
				b[w] = '\n'
				r++
				w++
			case 'r':
				b[w] = '\r'
				r++
				w++
			case 't':
				b[w] = '\t'
				r++
				w++
			case 'u':
				r--
				rr := getu4(s[r:])
				if rr < 0 {
					p.errorf("invalid unicode escape sequence")
				}
				r += 6
				if utf16.IsSurrogate(rr) {
					rr1 := getu4(s[r:])
					if dec := utf16.DecodeRune(rr, rr1); dec != unicode.ReplacementChar {
						// A valid pair; consume.
						r += 6
						w += utf8.EncodeRune(b[w:], dec)
						break
					}
					// Invalid surrogate; fall back to replacement rune.
					rr = unicode.ReplacementChar
				}
				w += utf8.EncodeRune(b[w:], rr)
			}

		// Quote, control characters are invalid.
		// Quote should not happen since the lexer would have dealt with it,
		// but have this just to be safe
		case c == '"', c < ' ':
			p.errorf("invalid character in string: %c", s[r])

		// ASCII
		case c < utf8.RuneSelf:
			b[w] = c
			r++
			w++

		// Coerce to well-formed UTF-8.
		default:
			rr, size := utf8.DecodeRune(s[r:])
			r += size
			w += utf8.EncodeRune(b[w:], rr)
		}
	}
	return string(b[0:w])
}

// getu4 decodes \uXXXX from the beginning of s, returning the hex value,
// or it returns -1.
func getu4(s []byte) rune {
	if len(s) < 6 || s[0] != '\\' || s[1] != 'u' {
		return -1
	}
	var r rune
	for _, c := range s[2:6] {
		switch {
		case '0' <= c && c <= '9':
			c = c - '0'
		case 'a' <= c && c <= 'f':
			c = c - 'a' + 10
		case 'A' <= c && c <= 'F':
			c = c - 'A' + 10
		default:
			return -1
		}
		r = r*16 + rune(c)
	}
	return r
}
