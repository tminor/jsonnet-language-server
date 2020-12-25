package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/tminor/jsonnet-language-server/pkg/analysis/lexical/token"
	"github.com/tminor/jsonnet-language-server/pkg/lsp"
	"github.com/tminor/jsonnet-language-server/pkg/tracing"
	"github.com/tminor/jsonnet-language-server/pkg/util/uri"
	"github.com/opentracing/opentracing-go/log"
	"github.com/pkg/errors"
)

const (
	// JsonnetLibPaths are jsonnet lib paths.
	JsonnetLibPaths = "jsonnet.libPaths"

	// TextDocumentUpdates are text document updates.
	TextDocumentUpdates = "textDocument.update"
)

// Config is configuration setting for the server.
type Config struct {
	textDocuments   map[string]TextDocument
	jsonnetLibPaths []string
	nodeCache       *token.NodeCache
	dispatchers     map[string]*Dispatcher
}

// New creates an instance of Config.
func New() *Config {
	return &Config{
		textDocuments:   make(map[string]TextDocument),
		jsonnetLibPaths: make([]string, 0),
		nodeCache:       token.NewNodeCache(),
		dispatchers:     map[string]*Dispatcher{},
	}
}

// NodeCache returns the node cache.
func (c *Config) NodeCache() *token.NodeCache {
	return c.nodeCache
}

// JsonnetLibPaths returns Jsonnet lib paths.
func (c *Config) JsonnetLibPaths() []string {
	return c.jsonnetLibPaths
}

// StoreTextDocumentItem stores a text document item.
func (c *Config) StoreTextDocumentItem(ctx context.Context, td TextDocument) error {
	span, ctx := tracing.ChildSpan(ctx, "storeTextDocument")
	defer span.Finish()

	oldDoc, ok := c.textDocuments[td.uri]
	if !ok {
		oldDoc = td
	}

	span.LogFields(
		log.String("textdocument.store", td.uri),
	)

	oldDoc.text = td.text
	oldDoc.version = td.version

	c.textDocuments[td.uri] = td
	c.dispatch(ctx, TextDocumentUpdates, td)
	return nil
}

// UpdateTextDocumentItem updates a text document item with a change event.
func (c *Config) UpdateTextDocumentItem(ctx context.Context, dctdp lsp.DidChangeTextDocumentParams) error {
	// The language server is configured to request for full content changes,
	// so the text in the change event is a full document.

	td := TextDocument{
		text:    dctdp.ContentChanges[0].Text,
		uri:     dctdp.TextDocument.URI,
		version: dctdp.TextDocument.Version,
	}

	return c.StoreTextDocumentItem(ctx, td)
}

// Text retrieves text from our local cache or from the file system.
func (c *Config) Text(ctx context.Context, uriStr string) (*TextDocument, error) {
	span, ctx := tracing.ChildSpan(ctx, "retrieveText")
	defer span.Finish()

	text, ok := c.textDocuments[uriStr]
	if ok {
		span.LogFields(
			log.String("config.retrieveFromCache", uriStr),
		)

		return &text, nil
	}

	span.LogFields(
		log.String("config.retrieveFromFS", uriStr),
	)

	path, err := uri.ToPath(uriStr)
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	td := &TextDocument{
		text: string(data),
	}

	return td, nil
}

// Watch will call `fn`` when key `k` is updated. It returns a
// cancel function.
func (c *Config) Watch(k string, fn DispatchFn) DispatchCancelFn {
	d := c.dispatcher(k)
	return d.Watch(fn)
}

func (c *Config) dispatcher(k string) *Dispatcher {
	d, ok := c.dispatchers[k]
	if !ok {
		d = NewDispatcher()
		c.dispatchers[k] = d
	}

	return d
}

func (c *Config) dispatch(ctx context.Context, k string, msg interface{}) {
	d := c.dispatcher(k)
	d.Dispatch(ctx, msg)
}

// UpdateClientConfiguration updates the configuration.
func (c *Config) UpdateClientConfiguration(ctx context.Context, update map[string]interface{}) error {
	for k, v := range update {
		switch k {
		case JsonnetLibPaths:
			paths, err := interfaceToStrings(v)
			if err != nil {
				return errors.Wrapf(err, "setting %q", JsonnetLibPaths)
			}

			c.jsonnetLibPaths = paths
			c.dispatch(ctx, JsonnetLibPaths, paths)
		default:
			return errors.Errorf("setting %q is unknown to the jsonnet language server", k)
		}
	}
	return nil
}

func (c *Config) String() string {
	data, err := c.MarshalJSON()
	if err != nil {
		panic(fmt.Sprintf("marshaling config to JSON: %v", err))
	}
	return string(data)
}

type configMarshaled struct {
	JsonnetLibPaths []string
}

// MarshalJSON marshals a config to JSON bytes.
func (c *Config) MarshalJSON() ([]byte, error) {
	cm := configMarshaled{
		JsonnetLibPaths: c.JsonnetLibPaths(),
	}

	return json.Marshal(&cm)
}

func interfaceToStrings(v interface{}) ([]string, error) {
	switch v := v.(type) {
	case []interface{}:
		var out []string
		for _, item := range v {
			str, ok := item.(string)
			if !ok {
				return nil, errors.Errorf("item was not a string")
			}

			out = append(out, str)
		}

		return out, nil
	case []string:
		return v, nil
	default:
		return nil, errors.Errorf("unable to convert %T to array of strings", v)
	}
}
