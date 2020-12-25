package lexical

import (
	"context"

	"github.com/tminor/jsonnet-language-server/pkg/config"
	"github.com/pkg/errors"
	"github.com/sourcegraph/jsonrpc2"
)

// RPCConn is a RPC server connection.
type RPCConn interface {
	Notify(ctx context.Context, method string, params interface{}, opts ...jsonrpc2.CallOption) error
}

// TextDocumentWatcherConfig is configuration for TextDocumentWatcher.
type TextDocumentWatcherConfig interface {
	Watch(string, config.DispatchFn) config.DispatchCancelFn
}

// TextDocumentWatcher watches text documents.
type TextDocumentWatcher struct {
	config            TextDocumentWatcherConfig
	documentProcessor DocumentProcessor
	conn              RPCConn
}

// NewTextDocumentWatcher creates an instance of NewTextDocumentWatcher.
func NewTextDocumentWatcher(c TextDocumentWatcherConfig, dp DocumentProcessor) *TextDocumentWatcher {
	tdw := &TextDocumentWatcher{
		config:            c,
		documentProcessor: dp,
	}

	c.Watch(config.TextDocumentUpdates, tdw.watch)

	return tdw
}

func (tdw *TextDocumentWatcher) SetConn(conn RPCConn) {
	tdw.conn = conn
}

func (tdw *TextDocumentWatcher) watch(ctx context.Context, item interface{}) error {
	tdi, ok := item.(config.TextDocument)
	if !ok {
		return errors.Errorf("text document watcher can't handle %T", item)
	}

	return tdw.documentProcessor.Process(ctx, tdi, tdw.conn)
}
