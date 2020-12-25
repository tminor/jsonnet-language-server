package token

import (
	"context"
	"fmt"

	jpos "github.com/tminor/jsonnet-language-server/pkg/util/position"
	"github.com/google/go-jsonnet/ast"
	opentracing "github.com/opentracing/opentracing-go"
)

// Highlight returns locations to highlight given source and a position.
func Highlight(ctx context.Context, filepath, source string, pos jpos.Position, nodeCache *NodeCache) (*jpos.Locations, error) {
	span, _ := opentracing.StartSpanFromContext(ctx, "highlight")
	defer span.Finish()

	node, err := ReadSource(filepath, source, nil)
	if err != nil {
		return nil, err
	}

	sg := scanScope(node, nodeCache)

	_, s, err := sg.at(pos)
	if err != nil {
		return nil, err
	}

	locations := s.refAt(pos)
	return &locations, nil

	// spew.Dump(pos, s.refMap)

	// id, path := idNode(found, pos, s)

	// return s.refersTo(id, path...), nil
}

func idNode(node ast.Node, pos jpos.Position, s *scope) (ast.Identifier, []string) {
	var id ast.Identifier
	var path []string
	switch found := node.(type) {
	case *ast.DesugaredObject:
		return idNode(s.parent(found), pos, s)
	case *ast.Function:
		for paramID, loc := range found.Parameters.RequiredLocs {
			if pos.IsInJsonnetRange(loc) {
				id = paramID
				break
			}
		}

		for _, param := range found.Parameters.Optional {
			if pos.IsInJsonnetRange(param.Loc) {
				id = param.Name
				break
			}
		}
	case *ast.Index:
		indexPath := resolveIndex(found)
		id = ast.Identifier(indexPath[0])
		path = indexPath[1:]
	case *ast.Local:
		for _, bind := range found.Binds {
			if pos.IsInJsonnetRange(bind.VarLoc) {
				id = bind.Variable
			}
		}
	case *ast.Var:
		id = found.Id
	default:
		panic(fmt.Sprintf("unable to find nodes of type %T", found))
	}

	return id, path
}
