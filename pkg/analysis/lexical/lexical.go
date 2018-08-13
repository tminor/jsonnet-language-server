package lexical

import (
	"io"

	"github.com/bryanl/jsonnet-language-server/pkg/analysis/lexical/locate"
	"github.com/google/go-jsonnet/ast"
	"github.com/pkg/errors"
	"github.com/sourcegraph/go-langserver/pkg/lsp"
)

var (
	emptyHover = &lsp.Hover{}
)

// TokenAtLocation returns the token a location in a file.
func TokenAtLocation(filename string, r io.Reader, loc ast.Location) (*locate.Locatable, error) {
	v, err := NewCursorVisitor(filename, r, loc)
	if err != nil {
		return nil, errors.Wrap(err, "create cursor visitor")
	}

	if err = v.Visit(); err != nil {
		return nil, errors.Wrap(err, "visit tokens")
	}

	locatable, err := v.TokenAtPosition()
	if err != nil {
		return nil, errors.Wrap(err, "find token at position")
	}

	return locatable, nil
}

func HoverAtLocation(filename string, r io.Reader, l, c int) (*lsp.Hover, error) {
	loc := ast.Location{
		Line:   l,
		Column: c,
	}

	v, err := newHoverVisitor(filename, r, loc)
	if err != nil {
		return nil, err
	}

	locatable, err := v.TokenAtLocation()
	if err != nil {
		return nil, err
	}

	if locatable == nil {
		return emptyHover, nil
	}

	resolved, err := locatable.Resolve()
	if err != nil {
		if err == locate.ErrUnresolvable {
			return emptyHover, nil
		}
		return nil, err
	}

	response := &lsp.Hover{
		Contents: []lsp.MarkedString{
			{
				Language: "jsonnet",
				Value:    resolved.Description,
			},
		},
		Range: lsp.Range{
			Start: lsp.Position{
				Line:      resolved.Location.Begin.Line - 1,
				Character: resolved.Location.Begin.Column - 1,
			},
			End: lsp.Position{
				Line:      resolved.Location.End.Line - 1,
				Character: resolved.Location.End.Column - 1,
			},
		},
	}

	return response, nil
}
