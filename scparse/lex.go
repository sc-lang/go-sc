// Copyright (c) 2021 the SC authors. All rights reserved. MIT License.

package scparse

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// tokenType identifies the type of a lex token.
type tokenType int

const (
	tokenError tokenType = iota // val contains error details
	tokenEOF
	tokenBool       // bool literal, either true or false
	tokenNumber     // number literal
	tokenString     // double quoted string (excludes quotes)
	tokenRawString  // raw quoted string (includes quotes)
	tokenIdentifier // alphanumberic identifier starting with a letter
	tokenComment    // a comment, either // or /* style
	// Everything from here on is a symbol or keyword
	tokenSymbol           // only used as a delimiter for token types
	tokenLeftSquareParen  // [
	tokenRightSquareParen // ]
	tokenLeftCurlyParen   // {
	tokenRightCurlyParen  // }
	tokenVariableStart    // ${
	tokenQuote            // "
	tokenColon            // :
	tokenComma            // ,
	tokenNull             // the null literal
)

// String returns a string representation of the token type.
// Useful for printing while debugging or in tests.
func (typ tokenType) String() string {
	return [...]string{
		"Error",
		"EOF",
		"Bool",
		"Number",
		"String",
		"RawString",
		"Identifier",
		"Comment",
		"Symbol", // Unused but required so the index works
		"LeftSquareParen",
		"RightSquareParen",
		"LeftCurlyParen",
		"RightCurlyParen",
		"VariableStart",
		"Quote",
		"Colon",
		"Comma",
		"Null",
	}[typ]
}

// token represents a token that was scanned by the lexer.
type token struct {
	typ tokenType // The type of this token.
	pos Pos       // The position of this token in the input text.
	val string    // The value of this token.
}

// String returns a string representation of the token.
func (t token) String() string {
	switch {
	case t.typ == tokenEOF:
		return "EOF"
	case t.typ == tokenError:
		return t.val
	case t.typ > tokenSymbol:
		return fmt.Sprintf("<%s>", t.val)
	case tokenString <= t.typ && t.typ <= tokenComment && len(t.val) > 10:
		// These values can get rather long so truncate them
		return fmt.Sprintf("<%s: %.10q>...", t.typ, t.val)
	}
	return fmt.Sprintf("<%s: %q>", t.typ, t.val)
}

const eof = -1

// stateFn represents the state of the lexer as a function that returns the next state.
// If the returned state is nil it indicates that the lexer has reached a terminal state
// and should end scanning.
type stateFn func(*lexer) stateFn

// lexerMode identifies the current mode of a lexer.
// Different modes can be used to tokenize input differently.
type lexerMode int

const (
	// lex as normal, nothing special
	lexerModeNormal lexerMode = iota
	// used when inside a string to handle variable interpolation
	lexerModeString
)

// lexer holds the state of the scanner.
type lexer struct {
	input       []byte     // the text being scanned
	pos         int        // current byte position in the input
	start       int        // start position of this token
	width       int        // width of the last rune read from input
	tokens      chan token // channel of scanned tokens
	line        int        // 1 + number of newlines seen
	startLine   int        // start line of this token
	insertComma bool       // should insert a comma before next newline
	mode        lexerMode  // the mode the lexer is currently in
}

// next returns the next rune in the input.
func (l *lexer) next() rune {
	if l.pos >= len(l.input) {
		l.width = 0
		return eof
	}

	r, w := utf8.DecodeRune(l.input[l.pos:])
	l.width = w
	l.pos += l.width
	if r == '\n' {
		l.line++
	}
	return r
}

// backup steps back one rune. Can only be called once per call of next.
func (l *lexer) backup() {
	l.pos -= l.width
	// Revert newline count if necessary
	if l.width == 1 && l.input[l.pos] == '\n' {
		l.line--
	}
}

// peek returns but does not consume the next rune in the input.
func (l *lexer) peek() rune {
	r := l.next()
	l.backup()
	return r
}

// tokenPos calculates the position of the current token that's about to be emitted.
func (l *lexer) tokenPos() Pos {
	// Need to calculate the column position
	// move to the byte after the newline
	// this also handles the first line since -1 + 1 = 0
	lineStart := bytes.LastIndexByte(l.input[:l.start], '\n') + 1
	// Count runes not bytes
	// +1 because it's 1-indexed
	col := utf8.RuneCount(l.input[lineStart:l.start]) + 1
	return Pos{
		Line:   l.startLine,
		Column: col,
		Byte:   l.start,
	}
}

// emit passes a token back to the client.
func (l *lexer) emit(t tokenType) {
	l.tokens <- token{
		typ: t,
		pos: l.tokenPos(),
		val: string(l.input[l.start:l.pos]),
	}
	l.start = l.pos
	l.startLine = l.line
}

// emitAutomaticComma performs automatic comma insertion by emitting a comma token.
// Unlike emit, it will not update start and startLine.
// If insertComma is false, this method will no-op.
func (l *lexer) emitAutomaticComma() {
	if !l.insertComma {
		return
	}
	l.insertComma = false
	l.tokens <- token{
		typ: tokenComma,
		pos: l.tokenPos(),
		// Make debugging easier by highlighting that this is an automatic comma.
		val: "automatic ,",
	}
}

// ignore skips over the pending input before this point.
func (l *lexer) ignore() {
	l.line += bytes.Count(l.input[l.start:l.pos], []byte{'\n'})
	l.start = l.pos
	l.startLine = l.line
}

// accept consumes the next rune if it's from the valid set.
func (l *lexer) accept(validRunes string) bool {
	if strings.ContainsRune(validRunes, l.next()) {
		return true
	}
	l.backup()
	return false
}

// acceptRun consumes a run of runes in the input that are in the valid set.
func (l *lexer) acceptRun(validSet string) {
	for strings.ContainsRune(validSet, l.next()) {
	}
	l.backup()
}

// errorf returns an error token and terminates the scan by passing back
// a nil pointer that will be the next state, terminating l.nextToken.
func (l *lexer) errorf(format string, args ...interface{}) stateFn {
	l.tokens <- token{
		typ: tokenError,
		pos: l.tokenPos(),
		val: fmt.Sprintf(format, args...),
	}
	return nil
}

// nextToken returns the next token from the input.
// Called by the parser, not in the lexing goroutine.
func (l *lexer) nextToken() token {
	return <-l.tokens
}

// drain drains the output so the lexing goroutine will exit.
// Called by the parser, not in the lexing goroutine.
func (l *lexer) drain() {
	for range l.tokens {
	}
}

// run runs the state machine for the lexer.
func (l *lexer) run() {
	for state := lexText; state != nil; {
		state = state(l)
	}
	close(l.tokens)
}

// lex creates a new scanner for the input text.
func lex(input []byte) *lexer {
	l := &lexer{
		input:     input,
		tokens:    make(chan token),
		line:      1,
		startLine: 1,
	}
	go l.run()
	return l
}

// state functions

// lexText scans until it can start matching a token.
// It is the initial state that is entered either when the lexer
// starts, or after it has emitted a token.
func lexText(l *lexer) stateFn {
	if l.mode == lexerModeString {
		return lexString
	}
	switch r := l.next(); {
	case r == eof: // end of file, end scanning
		l.emit(tokenEOF)
		return nil
	case isSpace(r) || isEndOfLine(r):
		l.backup() // backup in case it was a newline
		return lexSpace
	case r == '/':
		return lexComment
	case r == ':':
		l.emit(tokenColon)
		// Make sure we don't try inserting a comma after a key
		l.insertComma = false
	case r == ',':
		l.emit(tokenComma)
		// Explicit comma provided
		l.insertComma = false
	case r == '[':
		l.emit(tokenLeftSquareParen)
	case r == ']':
		l.emit(tokenRightSquareParen)
		l.insertComma = true
	case r == '{':
		l.emit(tokenLeftCurlyParen)
	case r == '}':
		l.emit(tokenRightCurlyParen)
		l.insertComma = true
	case r == '"':
		return lexQuote
	case r == '`':
		return lexRawQuote
	case r == '$':
		return lexVariable
	case r == '-' || ('0' <= r && r <= '9'):
		l.backup()
		return lexNumber
	case isAlphaNumeric(r):
		l.backup()
		return lexIdentifier
	default:
		return l.errorf("unrecognized character scanned: %#U", r)
	}
	return lexText
}

// lexSpace scans and ignores a sequence of whitespace or newlines.
func lexSpace(l *lexer) stateFn {
	for {
		r := l.peek()
		if isEndOfLine(r) {
			// Don't use next since it also counts newlines
			// Newlines will be counted at the end when calling l.ignore
			l.pos++
			l.emitAutomaticComma()
			continue
		}
		if !isSpace(r) {
			break
		}
		l.next()
	}
	l.ignore()
	return lexText
}

// lexComment scans a comment, either line or block style.
func lexComment(l *lexer) stateFn {
	// Consume first /
	r := l.next()
	// Figure out what type of comment
	switch r {
	case '/':
		return lexLineComment
	case '*':
		return lexBlockComment
	}
	return l.errorf("unrecognized sequence scanned: /%c", r)
}

// lexLineComment scans a //-stlye comment.
func lexLineComment(l *lexer) stateFn {
	// Guaranteed to have a newline or eof so just insert comma now
	l.emitAutomaticComma()

	// Find end of the line which is the end of the comment
	i := bytes.IndexByte(l.input[l.pos:], '\n')
	hasNL := true
	if i < 0 {
		// Singleline with no newline before eof, just scan until eof
		i = len(l.input[l.pos:])
		hasNL = false
	}
	l.pos += i
	l.emit(tokenComment)
	// ignore newline, make sure not EOF
	if hasNL {
		l.pos++
		l.ignore()
	}
	return lexText
}

// lexBlockComment scans a /*-style comment.
func lexBlockComment(l *lexer) stateFn {
	const commentEnd = "*/"
	i := bytes.Index(l.input[l.pos:], []byte(commentEnd))
	if i < 0 {
		return l.errorf("unclosed block comment")
	}
	l.pos += i + len(commentEnd)
	nline := bytes.Count(l.input[l.start:l.pos], []byte{'\n'})
	if nline > 0 {
		l.emitAutomaticComma()
	}
	l.line += nline
	l.emit(tokenComment)
	return lexText
}

// lexQuote scans a double quoted string. The lexer will enter string mode.
// The opening quote has already been scanned.
func lexQuote(l *lexer) stateFn {
	l.emit(tokenQuote)
	if l.mode == lexerModeNormal {
		l.mode = lexerModeString
		return lexString
	}
	l.mode = lexerModeNormal
	l.insertComma = true
	return lexText
}

// lexString scans a string. It expects the lexer to be in string mode.
func lexString(l *lexer) stateFn {
	// Invariant check, just to be safe
	if l.mode != lexerModeString {
		panic("lexString called with lexer not in string mode")
	}
	// Treat } specially if it's the first rune since we likely
	// just came from scanning a variable.
	// If it's a false positive, the parser will fix it
	if l.peek() == '}' {
		l.next()
		l.emit(tokenRightCurlyParen)
	}

Loop:
	for {
		switch l.next() {
		case '\\':
			// Handle escape character
			r := l.next()
			if r != eof && r != '\n' {
				break
			}
			fallthrough
		case eof, '\n':
			return l.errorf("unterminated string")
		case '"', '$':
			l.backup()
			break Loop
		}
	}
	// Emit string token if not empty
	if l.pos > l.start {
		l.emit(tokenString)
	}

	// Handle special char
	switch r := l.next(); r {
	case '"':
		return lexQuote
	case '$':
		return lexVariable
	default:
		return l.errorf("unrecognized character scanned: %#U", r)
	}
}

// lexRawQuote scans a raw quoted string.
// The opening quote has already been scanned.
func lexRawQuote(l *lexer) stateFn {
Loop:
	for {
		switch l.next() {
		case eof:
			return l.errorf("unterminated raw string")
		case '`':
			break Loop
		}
	}
	l.emit(tokenRawString)
	l.insertComma = true
	return lexText
}

// lexNumber scans a number: int or float.
func lexNumber(l *lexer) stateFn {
	// Optional negative sign
	l.accept("-")
	digits := "0123456789"
	l.acceptRun(digits)
	// Handle float
	if l.accept(".") {
		l.acceptRun(digits)
	}
	// Handle exponent
	if l.accept("eE") {
		l.accept("+-")
		l.acceptRun(digits)
	}
	// Next thing must not be alphanumeric or it's
	// a malformed number
	if isAlphaNumeric(l.peek()) {
		l.next()
		return l.errorf("bad number syntax: %q", l.input[l.start:l.pos])
	}
	l.emit(tokenNumber)
	l.insertComma = true
	return lexText
}

// lexIdentifier scans an alphanumeric identifier.
func lexIdentifier(l *lexer) stateFn {
	for {
		r := l.next()
		if isAlphaNumeric(r) {
			// consume and keep going
			continue
		}
		l.backup()
		word := string(l.input[l.start:l.pos])
		if !l.atTerminator() {
			return l.errorf("bad character %#U in identifier", r)
		}
		// Check if it matches a keyword
		switch word {
		case "null":
			l.emit(tokenNull)
		case "true", "false":
			l.emit(tokenBool)
		default:
			l.emit(tokenIdentifier)
		}
		break
	}
	l.insertComma = true
	return lexText
}

// lexVariable scans a variable. The $ has been scanned.
func lexVariable(l *lexer) stateFn {
	// Variable start is `${`
	if r := l.next(); r != '{' {
		// If in string mode then this is not actually a variable,
		// just a $ character on its own
		if l.mode == lexerModeString {
			l.backup()
			return lexString
		}
		return l.errorf("bad character %#U after '$', expected '{'", r)
	}
	l.emit(tokenVariableStart)
	// return lexFieldOrVariable(l, tokenIdentifier)
	// If at terminator this is a lexical error
	// We must have a name after a variable or field
	if l.atTerminator() {
		return l.errorf("variable name missing after '${'")
	}

	// Scan variable name
	// First rune in the variable name must be a letter
	r := l.next()
	if r != '_' && !unicode.IsLetter(r) {
		return l.errorf("bad character %#U after '${'", r)
	}
	for {
		r = l.next()
		if !isAlphaNumeric(r) {
			l.backup()
			break
		}
	}
	if !l.atTerminator() {
		return l.errorf("bad character %#U", r)
	}
	l.emit(tokenIdentifier)
	return lexText
}

// atTerminator reports whether the input is at a valid termination
// character after an identifier.
func (l *lexer) atTerminator() bool {
	r := l.peek()
	if isSpace(r) || isEndOfLine(r) {
		return true
	}
	switch r {
	case eof, ',', ':', '.', ']', '}':
		return true
	}
	return false
}

// util functions

// isSpace reports whether r is a space char.
func isSpace(r rune) bool {
	return r == ' ' || r == '\t'
}

// isEndOfLine reports whether r is an end-of-line char.
func isEndOfLine(r rune) bool {
	return r == '\r' || r == '\n'
}

// isAlphaNumeric reports whether r is an alphabetic, digit, or underscore.
func isAlphaNumeric(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
