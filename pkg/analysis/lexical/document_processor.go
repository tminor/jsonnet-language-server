package lexical

import (
	"context"

	"github.com/tminor/jsonnet-language-server/pkg/analysis/lexical/token"
	"github.com/tminor/jsonnet-language-server/pkg/config"
	"github.com/tminor/jsonnet-language-server/pkg/lsp"
	"github.com/tminor/jsonnet-language-server/pkg/tracing"
	"github.com/tminor/jsonnet-language-server/pkg/util/position"
	"github.com/tminor/jsonnet-language-server/pkg/util/uri"
	"github.com/google/go-jsonnet/ast"
	"github.com/opentracing/opentracing-go/log"
	"github.com/pkg/errors"
)

// DocumentProcessor processes TextDocument.
type DocumentProcessor interface {
	Process(ctx context.Context, td config.TextDocument, conn RPCConn) error
}

// PerformDiagnostics performs diagnostics on a text document and sends results
// to the client.
type PerformDiagnostics struct{}

var _ DocumentProcessor = (*PerformDiagnostics)(nil)

// NewPerformDiagnostics creates an instance of PerformDiagnostics.
func NewPerformDiagnostics() *PerformDiagnostics {
	return &PerformDiagnostics{}
}

// Process runs the diagnositics.
func (p *PerformDiagnostics) Process(ctx context.Context, td config.TextDocument, conn RPCConn) error {
	span, _ := tracing.ChildSpan(ctx, "performDiagnostics")
	defer span.Finish()

	span.LogFields(
		log.String("caching", td.URI()),
	)

	filename, err := uri.ToPath(td.URI())
	if err != nil {
		return err
	}

	done := make(chan bool, 1)
	diagCh := make(chan token.ParseDiagnostic, 1)

	diagnostics := make([]lsp.Diagnostic, 0)

	go func() {
		if diagCh == nil {
			close(done)
			return
		}

		for d := range diagCh {
			if conn != nil {
				r := position.FromJsonnetRange(d.Loc)

				diagnostic := lsp.Diagnostic{
					Range:    r.ToLSP(),
					Message:  d.Message,
					Severity: lsp.Error,
				}

				diagnostics = append(diagnostics, diagnostic)
			}
		}

		close(done)
	}()

	_, err = convertToNode(filename, td.String(), diagCh)
	if err != nil {
		return errors.Wrap(err, "converting source to node")
	}

	<-done

	if conn != nil {
		span.LogFields(
			log.String("event", "sending diagnostics"),
		)

		response := &lsp.PublishDiagnosticsParams{
			URI:         td.URI(),
			Diagnostics: diagnostics,
		}

		ctx := context.Background()
		method := "textDocument/publishDiagnostics"
		if err := conn.Notify(ctx, method, response); err != nil {
			span.LogFields(
				log.Error(err),
			)

		}

	}

	return nil
}

func convertToNode(filename, snippet string, diagCh chan<- token.ParseDiagnostic) (ast.Node, error) {
	node, err := token.Parse(filename, snippet, diagCh)
	if err != nil {
		return nil, errors.Wrap(err, "parsing source")
	}

	if err := token.DesugarFile(&node); err != nil {
		return nil, err
	}

	return node, nil
}
