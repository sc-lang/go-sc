// Copyright (c) 2021 the SC authors. All rights reserved. MIT License.

package scparse

import "testing"

type lexTest struct {
	name   string
	input  string
	tokens []token
}

var (
	tEOF       = mkToken(tokenEOF, "")
	tLsquare   = mkToken(tokenLeftSquareParen, "[")
	tRsquare   = mkToken(tokenRightSquareParen, "]")
	tLcurly    = mkToken(tokenLeftCurlyParen, "{")
	tRcurly    = mkToken(tokenRightCurlyParen, "}")
	tVarStart  = mkToken(tokenVariableStart, "${")
	tQuote     = mkToken(tokenQuote, `"`)
	tColon     = mkToken(tokenColon, ":")
	tComma     = mkToken(tokenComma, ",")
	tAutoComma = mkToken(tokenComma, "automatic ,")
	tNull      = mkToken(tokenNull, "null")
)

func mkToken(typ tokenType, val string) token {
	return token{typ: typ, val: val}
}

// collectTokens gathers the emitted tokens into a slice.
func collectTokens(t *lexTest) []token {
	l := lex([]byte(t.input))
	var tokens []token
	for {
		tok := l.nextToken()
		tokens = append(tokens, tok)
		if tok.typ == tokenEOF || tok.typ == tokenError {
			break
		}
	}
	return tokens
}

func TestLex(t *testing.T) {
	tests := []lexTest{
		// individual tokens
		{"empty", "", []token{tEOF}},
		{"whitespace", "  \t\n\r   ", []token{tEOF}},
		{"keywords", "true false null", []token{
			mkToken(tokenBool, "true"),
			mkToken(tokenBool, "false"),
			tNull,
			tEOF,
		}},
		{"string", `"foo\t bar\"\n"`, []token{
			tQuote,
			mkToken(tokenString, `foo\t bar\"\n`),
			tQuote,
			tEOF,
		}},
		{"interpolated string", `"/foo/${path}/bar"`, []token{
			tQuote,
			mkToken(tokenString, "/foo/"),
			tVarStart,
			mkToken(tokenIdentifier, "path"),
			tRcurly,
			mkToken(tokenString, "/bar"),
			tQuote,
			tEOF,
		}},
		{"raw string", "`foo\\t bar\\\"\\n`", []token{
			mkToken(tokenRawString, "`foo\\t bar\\\"\\n`"), tEOF,
		}},
		{"raw string with newline", "`a multi\nline string`", []token{
			mkToken(tokenRawString, "`a multi\nline string`"), tEOF,
		}},
		{"parens", "{[]}", []token{tLcurly, tLsquare, tRsquare, tRcurly, tEOF}},
		{"symbols", ":,", []token{tColon, tComma, tEOF}},
		{"numbers", "24 -42 0000.1756 13.79 1E3 1.5e-3 7e+5", []token{
			mkToken(tokenNumber, "24"),
			mkToken(tokenNumber, "-42"),
			mkToken(tokenNumber, "0000.1756"),
			mkToken(tokenNumber, "13.79"),
			mkToken(tokenNumber, "1E3"),
			mkToken(tokenNumber, "1.5e-3"),
			mkToken(tokenNumber, "7e+5"),
			tEOF,
		}},
		{"variables", "${foo} ${envs}", []token{
			tVarStart,
			mkToken(tokenIdentifier, "foo"),
			tRcurly,
			tVarStart,
			mkToken(tokenIdentifier, "envs"),
			tRcurly,
			tEOF,
		}},
		{"identifiers", "data _type foo123 a573bcd", []token{
			mkToken(tokenIdentifier, "data"),
			mkToken(tokenIdentifier, "_type"),
			mkToken(tokenIdentifier, "foo123"),
			mkToken(tokenIdentifier, "a573bcd"),
			tEOF,
		}},
		{"comments", "// single line\n/*1\n2\n3\n*/ /* more */ // hello", []token{
			mkToken(tokenComment, "// single line"),
			mkToken(tokenComment, "/*1\n2\n3\n*/"),
			mkToken(tokenComment, "/* more */"),
			mkToken(tokenComment, "// hello"),
			tEOF,
		}},
		// errors
		{"unrecognized char", ":@foo", []token{
			tColon,
			mkToken(tokenError, "unrecognized character scanned: U+0040 '@'"),
		}},
		{"invalid comment start", "/ hello", []token{
			mkToken(tokenError, "unrecognized sequence scanned: / "),
		}},
		{"unterminated comment", "/* this doesn't end", []token{
			mkToken(tokenError, "unclosed block comment"),
		}},
		{"unterminated string", `"this string doesn't end`, []token{
			tQuote, mkToken(tokenError, "unterminated string"),
		}},
		{"unterminated raw string", "`this raw string never ends", []token{
			mkToken(tokenError, "unterminated raw string"),
		}},
		{"bad number", "3n", []token{mkToken(tokenError, `bad number syntax: "3n"`)}},
		{"bad char in identifier", "foo12@3", []token{
			mkToken(tokenError, "bad character U+0040 '@' in identifier"),
		}},
		{"invalid var start", "$foo", []token{
			mkToken(tokenError, "bad character U+0066 'f' after '$', expected '{'"),
		}},
		{"missing var name", "${", []token{
			tVarStart, mkToken(tokenError, "variable name missing after '${'"),
		}},
		{"invalid var name", "${1abc}", []token{
			tVarStart, mkToken(tokenError, "bad character U+0031 '1' after '${'"),
		}},
		// more complex example
		{"complex config", `{
	foo: [true
	{ bar: 1.3, baz: "str val"},
				]
	"complex key": null}`, []token{
			tLcurly,
			mkToken(tokenIdentifier, "foo"),
			tColon,
			tLsquare,
			mkToken(tokenBool, "true"),
			tAutoComma,
			tLcurly,
			mkToken(tokenIdentifier, "bar"),
			tColon,
			mkToken(tokenNumber, "1.3"),
			tComma,
			mkToken(tokenIdentifier, "baz"),
			tColon,
			tQuote,
			mkToken(tokenString, "str val"),
			tQuote,
			tRcurly,
			tComma,
			tRsquare,
			tAutoComma,
			tQuote,
			mkToken(tokenString, "complex key"),
			tQuote,
			tColon,
			tNull,
			tRcurly,
			tEOF,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := collectTokens(&tt)
			if ok, diff := deepEqual(tokens, tt.tokens, "pos"); !ok {
				t.Errorf("tokens not equal:\n%s", diff)
			}
		})
	}
}

func TestLexPos(t *testing.T) {
	tests := []lexTest{
		{"empty", "", []token{
			{tokenEOF, Pos{1, 1, 0}, ""},
		}},
		{"symbols", `[{}]:,`, []token{
			{tokenLeftSquareParen, Pos{1, 1, 0}, "["},
			{tokenLeftCurlyParen, Pos{1, 2, 1}, "{"},
			{tokenRightCurlyParen, Pos{1, 3, 2}, "}"},
			{tokenRightSquareParen, Pos{1, 4, 3}, "]"},
			{tokenColon, Pos{1, 5, 4}, ":"},
			{tokenComma, Pos{1, 6, 5}, ","},
			{tokenEOF, Pos{1, 7, 6}, ""},
		}},
		{"multiline", "[true,\nfalse,\nnull\n]\n", []token{
			{tokenLeftSquareParen, Pos{1, 1, 0}, "["},
			{tokenBool, Pos{1, 2, 1}, "true"},
			{tokenComma, Pos{1, 6, 5}, ","},
			{tokenBool, Pos{2, 1, 7}, "false"},
			{tokenComma, Pos{2, 6, 12}, ","},
			{tokenNull, Pos{3, 1, 14}, "null"},
			{tokenComma, Pos{3, 5, 18}, "automatic ,"},
			{tokenRightSquareParen, Pos{4, 1, 19}, "]"},
			{tokenComma, Pos{4, 2, 20}, "automatic ,"},
			{tokenEOF, Pos{5, 1, 21}, ""},
		}},
		// check multi-byte runes
		{"emojis", `"😂abc"` + "\n" + `"foo🚀"`, []token{
			{tokenQuote, Pos{1, 1, 0}, `"`},
			{tokenString, Pos{1, 2, 1}, "😂abc"},
			{tokenQuote, Pos{1, 6, 8}, `"`},
			{tokenComma, Pos{1, 7, 9}, "automatic ,"},
			{tokenQuote, Pos{2, 1, 10}, `"`},
			{tokenString, Pos{2, 2, 11}, "foo🚀"},
			{tokenQuote, Pos{2, 6, 18}, `"`},
			{tokenEOF, Pos{2, 7, 19}, ""},
		}},
		// check errors have correct position
		{"error", ":\nnull @", []token{
			{tokenColon, Pos{1, 1, 0}, ":"},
			{tokenNull, Pos{2, 1, 2}, "null"},
			{tokenError, Pos{2, 6, 7}, "unrecognized character scanned: U+0040 '@'"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := collectTokens(&tt)
			if ok, diff := deepEqual(tokens, tt.tokens); !ok {
				t.Errorf("tokens not equal:\n%s", diff)
			}
		})
	}
}

// Test that an error shuts down the lexing goroutine.
func TestErrorShutdown(t *testing.T) {
	input := []byte(`{ foo: "`) // will cause lex error
	lexer := lex(input)
	_, err := parseLexer(input, lexer)
	if err == nil {
		t.Fatalf("expected error")
	}
	// The error should have drained the input. Therefore, the lexer should be shut down.
	token, ok := <-lexer.tokens
	if ok {
		t.Fatalf("lexer was not drained, got token %v", token)
	}
}

// parseLexer is like Parse but it lets us pass in the lexer instead of building it.
func parseLexer(input []byte, lex *lexer) (n *DictionaryNode, err error) {
	p := &parser{lex: lex}
	defer p.recover(&err)
	n = p.parse()
	p.lex = nil
	return n, nil
}
