package server

import (
	"strings"

	"github.com/bryanl/jsonnet-language-server/pkg/analysis/lexical"
	"github.com/bryanl/jsonnet-language-server/pkg/config"
	"github.com/bryanl/jsonnet-language-server/pkg/lsp"
	"github.com/bryanl/jsonnet-language-server/pkg/util/uri"
	"github.com/google/go-jsonnet/ast"
)

type hover struct {
	params lsp.TextDocumentPositionParams
	config *config.Config
}

func newHover(params lsp.TextDocumentPositionParams, c *config.Config) *hover {
	return &hover{
		params: params,
		config: c,
	}
}

func (h *hover) handle() (interface{}, error) {
	path, err := uri.ToPath(h.params.TextDocument.URI)
	if err != nil {
		return nil, err
	}

	text, err := h.config.Text(h.params.TextDocument.URI)
	if err != nil {
		return nil, err
	}

	r := strings.NewReader(text)

	return lexical.HoverAtLocation(path, r, h.params.Position.Line+1, h.params.Position.Character+1, h.config)
}

func posToLoc(pos lsp.Position) ast.Location {
	return ast.Location{
		Line:   pos.Line + 1,
		Column: pos.Character + 1,
	}
}
