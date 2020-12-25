package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"

	"github.com/tminor/jsonnet-language-server/pkg/analysis/lexical"
	"github.com/tminor/jsonnet-language-server/pkg/analysis/lexical/token"
	"github.com/tminor/jsonnet-language-server/pkg/config"
	"github.com/tminor/jsonnet-language-server/pkg/lsp"
	"github.com/tminor/jsonnet-language-server/pkg/tracing"
	"github.com/tminor/jsonnet-language-server/pkg/util/uri"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/sourcegraph/jsonrpc2"
	"go.uber.org/zap"
)

type operation func(context.Context, *request, *config.Config) (interface{}, error)

var operations = map[string]operation{
	"completionItem/resolve":         completionItemResolve,
	"initialize":                     initialize,
	"textDocument/completion":        textDocumentCompletion,
	"textDocument/didChange":         textDocumentDidChange,
	"textDocument/didClose":          textDocumentDidClose,
	"textDocument/didOpen":           textDocumentDidOpen,
	"textDocument/didSave":           textDocumentDidSave,
	"textDocument/documentHighlight": textDocumentHighlight,
	"textDocument/documentSymbol":    textDocumentSymbol,
	"textDocument/hover":             textDocumentHover,
	"textDocument/references":        textDocumentReferences,
	"textDocument/signatureHelp":     textDocumentSignatureHelper,
	"updateClientConfiguration":      updateClientConfiguration,
}

// Handler is a JSON RPC Handler
type Handler struct {
	zapLogger           *zap.Logger
	config              *config.Config
	decoder             *requestDecoder
	nodeCache           *token.NodeCache
	textDocumentWatcher *lexical.TextDocumentWatcher
	conn                *jsonrpc2.Conn
	tracer              opentracing.Tracer
	tracerCloser        io.Closer
}

var _ jsonrpc2.Handler = (*Handler)(nil)

// NewHandler creates a handler to handle rpc commands.
func NewHandler(zLogger *zap.Logger) *Handler {
	c := config.New()
	nodeCache := token.NewNodeCache()

	zapLogger := zLogger.With(zap.String("component", "handler"))

	tdw := lexical.NewTextDocumentWatcher(c, lexical.NewPerformDiagnostics())

	tracer, tracerCloser := initTracing("jsonnet-langauge-server", zapLogger)

	return &Handler{
		zapLogger:           zapLogger,
		decoder:             &requestDecoder{},
		config:              c,
		nodeCache:           nodeCache,
		textDocumentWatcher: tdw,
		tracer:              tracer,
		tracerCloser:        tracerCloser,
	}
}

// Close closes the handler.
func (h *Handler) Close() error {
	if h.tracerCloser != nil {
		return h.tracerCloser.Close()
	}

	return nil
}

// SetConn sets the RPC connection for the handler.
func (h *Handler) SetConn(conn *jsonrpc2.Conn) {
	h.conn = conn
	h.textDocumentWatcher.SetConn(conn)
}

type request struct {
	conn    *jsonrpc2.Conn
	req     *jsonrpc2.Request
	decoder *requestDecoder

	spanOnce sync.Once
}

func (r *request) Decode(v interface{}) error {
	return r.decoder.Decode(r.req, v)
}

func (r *request) RegisterCapability(ctx context.Context, method string, options interface{}) (string, error) {
	id := uuid.NewV4()

	registrations := &lsp.RegistrationParams{
		Registrations: []lsp.Registration{
			{
				ID:              id.String(),
				Method:          method,
				RegisterOptions: options,
			},
		},
	}

	var result interface{}

	if err := r.conn.Call(ctx, "client/registerCapability", registrations, result); err != nil {
		return "", err
	}

	return id.String(), nil
}

// Handle handles a JSON RPC connection.
func (lh *Handler) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	span := lh.tracer.StartSpan(req.Method)
	span.SetTag("id", req.ID.String())
	defer span.Finish()

	ctx = opentracing.ContextWithSpan(ctx, span)

	r := &request{
		conn: conn,
		req:  req,
	}

	defer func() {
		if r := recover(); r != nil {
			err := errors.Errorf("(CRASH) %v: %s", r, debug.Stack())
			log.Error(err)
		}
	}()

	fn, ok := operations[req.Method]
	if !ok {
		span.LogFields(
			log.String("error", fmt.Sprintf("unable to handle message type %s", string(*req.Params))),
		)
		return
	}

	response, err := fn(ctx, r, lh.config)
	if err != nil {
		span.LogFields(
			log.Error(err),
		)
		msg := &jsonrpc2.Error{
			Code:    jsonrpc2.CodeInternalError,
			Message: err.Error(),
		}
		if replyErr := conn.ReplyWithError(ctx, req.ID, msg); err != nil {
			span.LogFields(
				log.Error(replyErr),
			)

		}
		return
	}

	if err := conn.Reply(ctx, req.ID, response); err != nil {
		span.LogFields(
			log.Error(err),
		)
	}
}

func completionItemResolve(ctx context.Context, r *request, c *config.Config) (interface{}, error) {
	var ci lsp.CompletionItem
	if err := r.Decode(&ci); err != nil {
		return nil, err
	}

	// TODO figure out what do do here. This might not be needed and
	// it can drop.
	return nil, nil
}

func updateNodeCache(ctx context.Context, r *request, c *config.Config, uriStr string) {
	span, ctx := tracing.ChildSpan(ctx, "updateNodeCache")
	defer span.Finish()

	path, err := uri.ToPath(uriStr)
	if err != nil {
		span.LogFields(
			log.Error(err),
		)
		return
	}

	// do notification stuff here

	done := make(chan bool, 1)
	errCh := make(chan error, 1)

	timer := time.NewTimer(1 * time.Second)
	defer timer.Stop()

	go func() {
		err := token.UpdateNodeCache(ctx, path, c.JsonnetLibPaths(), c.NodeCache())
		if err != nil {
			errCh <- err
			return
		}

		done <- true
	}()

	sentNotif := false

	_, file := filepath.Split(path)

	for {
		select {
		case err := <-errCh:
			span.LogFields(
				log.String("uri", path),
				log.Error(err),
			)
			return
		case <-done:
			if sentNotif {
				msg := fmt.Sprintf("Import processing for %q is complete", file)
				_ = showMessage(ctx, r, lsp.MTWarning, msg)
				span.LogFields(
					log.String("status", "cancel notification"),
				)
			}
			return
		case <-timer.C:
			msg := fmt.Sprintf("Import processing for %q is running", file)
			_ = showMessage(ctx, r, lsp.MTWarning, msg)
			span.LogFields(
				log.String("status", "send notification"),
			)
			sentNotif = true
		}
	}

}

func closeFile(ctx context.Context, c *config.Config, uriStr string) {
	span := opentracing.SpanFromContext(ctx)

	path, err := uri.ToPath(uriStr)
	if err != nil {
		span.LogFields(
			log.Error(err),
		)
		return
	}

	nodeCache := c.NodeCache()
	if err := nodeCache.Remove(path); err != nil {
		span.LogFields(
			log.Error(err),
		)
	}
}

func textDocumentDidOpen(ctx context.Context, r *request, c *config.Config) (interface{}, error) {
	span := opentracing.SpanFromContext(ctx)
	ctx = opentracing.ContextWithSpan(ctx, span)

	var dotdp lsp.DidOpenTextDocumentParams
	if err := r.Decode(&dotdp); err != nil {
		return nil, err
	}

	span.LogFields(
		log.String("uri", dotdp.TextDocument.URI),
	)

	td := config.NewTextDocumentFromItem(dotdp.TextDocument)
	if err := c.StoreTextDocumentItem(ctx, td); err != nil {
		return nil, err
	}

	go updateNodeCache(ctx, r, c, dotdp.TextDocument.URI)

	return nil, nil
}

func textDocumentDidSave(ctx context.Context, r *request, c *config.Config) (interface{}, error) {
	span := opentracing.SpanFromContext(ctx)

	var dotdp lsp.DidOpenTextDocumentParams
	if err := r.Decode(&dotdp); err != nil {
		return nil, err
	}

	span.LogFields(
		log.String("uri", dotdp.TextDocument.URI),
	)

	go updateNodeCache(ctx, r, c, dotdp.TextDocument.URI)

	return nil, nil
}

func textDocumentDidClose(ctx context.Context, r *request, c *config.Config) (interface{}, error) {
	span := opentracing.SpanFromContext(ctx)

	var params lsp.DidCloseTextDocumentParams
	if err := r.Decode(&params); err != nil {
		return nil, err
	}

	span.LogFields(
		log.String("uri", params.TextDocument.URI),
	)

	go closeFile(ctx, c, params.TextDocument.URI)

	return nil, nil
}

func textDocumentDidChange(ctx context.Context, r *request, c *config.Config) (interface{}, error) {
	span := opentracing.SpanFromContext(ctx)
	ctx = opentracing.ContextWithSpan(ctx, span)

	var dctdp lsp.DidChangeTextDocumentParams
	if err := r.Decode(&dctdp); err != nil {
		return nil, err
	}

	if err := c.UpdateTextDocumentItem(ctx, dctdp); err != nil {
		return nil, err
	}

	return nil, nil
}

func showMessage(ctx context.Context, r *request, mt lsp.MessageType, message string) error {
	smp := &lsp.ShowMessageParams{
		Type:    int(mt),
		Message: message,
	}

	return r.conn.Notify(ctx, "window/showMessage", smp)
}

type requestDecoder struct {
}

func (rd *requestDecoder) Decode(req *jsonrpc2.Request, v interface{}) error {
	if err := json.Unmarshal(*req.Params, v); err != nil {
		return errors.Wrap(err, "invalid payload")
	}

	return nil
}
