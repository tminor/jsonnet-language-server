package server

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"

	"github.com/tminor/jsonnet-language-server/pkg/analysis/lexical/astext"
	"github.com/tminor/jsonnet-language-server/pkg/analysis/lexical/token"
	"github.com/tminor/jsonnet-language-server/pkg/config"
	"github.com/tminor/jsonnet-language-server/pkg/langserver"
	"github.com/tminor/jsonnet-language-server/pkg/lsp"
	"github.com/tminor/jsonnet-language-server/pkg/util/position"
	"github.com/tminor/jsonnet-language-server/pkg/util/text"
	"github.com/google/go-jsonnet/ast"
	"github.com/pkg/errors"
)

type jsonnetPathManager interface {
	Files() ([]string, error)
}

type defaultJsonnetPathManager struct {
	config *config.Config
}

func newJsonnetPathManager(c *config.Config) *defaultJsonnetPathManager {
	return &defaultJsonnetPathManager{
		config: c,
	}
}

func (jpm *defaultJsonnetPathManager) Files() ([]string, error) {
	lp := langserver.NewLibPaths(jpm.config.JsonnetLibPaths())
	return lp.Files()
}

type matchHandler struct {
	jsonnetPathManager jsonnetPathManager
	nodeCache          *token.NodeCache
}

func newMatchHandler(jpm jsonnetPathManager, nc *token.NodeCache) *matchHandler {
	mh := &matchHandler{
		jsonnetPathManager: jpm,
		nodeCache:          nc,
	}

	return mh
}

func (mh *matchHandler) register(cm *langserver.CompletionMatcher) error {
	m := map[string]langserver.CompletionAction{
		`import\s`:    mh.handleImport,
		`importstr\s`: mh.handleImport,
		`\w+\.`:       mh.handleIndex,
	}

	for term, fn := range m {
		if err := cm.Register(term, fn); err != nil {
			return errors.Wrapf(err, "registering completion matcher %q", term)
		}
	}

	return nil
}

func (mh *matchHandler) handleImport(ctx context.Context, pos position.Position, path, source string) ([]lsp.CompletionItem, error) {
	span := opentracing.SpanFromContext(ctx)
	span.LogFields(
		log.String("match.type", "import"),
	)

	editRange := position.NewRange(pos, pos)
	var items []lsp.CompletionItem

	files, err := mh.jsonnetPathManager.Files()
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		text := fmt.Sprintf(`"%s"`, file)
		ci := createCompletionItem(file, text, lsp.CIKFile, editRange, nil)
		items = append(items, ci)

	}

	return items, nil
}

func (mh *matchHandler) handleIndex(ctx context.Context, pos position.Position, filePath, source string) ([]lsp.CompletionItem, error) {
	span := opentracing.SpanFromContext(ctx)
	span.LogFields(
		log.String("match.type", "index"),
	)

	var items []lsp.CompletionItem

	scope, err := token.LocationScope(filePath, source, pos, mh.nodeCache)
	if err != nil {
		return nil, err
	}

	truncated, err := text.Truncate(source, pos)
	if err != nil {
		return nil, err
	}

	path, err := resolveIndex(truncated)
	if err != nil {
		return nil, err
	}

	se, err := scope.GetInPath(path)
	if err != nil {
		return nil, err
	}

	editRange := position.NewRange(pos, pos)

	switch n := se.Node.(type) {
	case *ast.DesugaredObject:
		for _, field := range n.Fields {
			name := astext.TokenValue(field.Name)

			fieldSe := &token.ScopeEntry{
				Detail: astext.TokenName(field.Body),
			}

			ci := createCompletionItem(name, name, lsp.CIKVariable, editRange, fieldSe)
			items = append(items, ci)
		}
	case *ast.Object:
		// TODO check to see if this case is used. Objects should be desguared.
		for _, field := range n.Fields {
			var name string
			switch field.Kind {
			case ast.ObjectFieldID:
				if field.Id == nil {
					return nil, errors.New("field id shouldn't be nil")
				}
				name = string(*field.Id)
			case ast.ObjectFieldStr:
				if field.Expr1 == nil {
					return nil, errors.New("field id should be a string")
				}
				name = astext.TokenValue(field.Expr1)
			}
			if name != "" {
				fieldSe := &token.ScopeEntry{
					Detail: astext.TokenName(field.Expr2),
				}

				ci := createCompletionItem(name, name, lsp.CIKVariable, editRange, fieldSe)
				items = append(items, ci)
			}
		}
	default:
		span.LogFields(
			log.String("text.unhandled", fmt.Sprintf("%T", n)),
		)
	}

	return items, nil
}

func createCompletionItem(label, text string, kind int, r position.Range, se *token.ScopeEntry) lsp.CompletionItem {
	var detail, documentation string
	if se != nil {
		detail = se.Detail
		documentation = se.Documentation
	}

	return lsp.CompletionItem{
		Label:         label,
		Kind:          kind,
		Detail:        detail,
		Documentation: documentation,
		TextEdit: lsp.TextEdit{
			Range:   r.ToLSP(),
			NewText: text,
		},
	}
}

var (
	reIndex = regexp.MustCompile(`((\w+\.)*\w+)\.[;\]\)\}]*$`)
)

func resolveIndex(source string) ([]string, error) {
	match := reIndex.FindAllString(source, 1)
	if match == nil {
		return nil, errors.Errorf("%q does not contain an index", source)
	}

	if len(match) != 1 {
		return nil, errors.Errorf("expected only one match when looking for index")
	}

	return removeEmpty(strings.Split(match[0], ".")), nil
}

var (
	ignoredIndexItems = []string{"}", "]", ")", ";"}
)

func removeEmpty(sl []string) []string {
	var out []string
	for _, s := range sl {
		if s != "" {
			if !stringInSlice(s, ignoredIndexItems) {
				out = append(out, s)
			}
		}
	}

	return out
}

func stringInSlice(s string, sl []string) bool {
	for i := range sl {
		if sl[i] == s {
			return true
		}
	}

	return false
}
