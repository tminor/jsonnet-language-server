package token

import (
	"testing"

	jpos "github.com/tminor/jsonnet-language-server/pkg/util/position"
	"github.com/google/go-jsonnet/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_objectMapper_lookup(t *testing.T) {
	var om objectMapper

	source := "{a: {b: 'c'}}"
	node, err := ReadSource("file.jsonnet", source, nil)
	require.NoError(t, err)

	o, ok := node.(*ast.DesugaredObject)
	require.True(t, ok)
	err = om.add(o, "a")
	require.NoError(t, err)
	l, err := om.lookup(o, []string{"a"})
	require.NoError(t, err)

	expected := jpos.NewLocation("file.jsonnet",
		jpos.NewRangeFromCoords(1, 2, 1, 3))
	assert.Equal(t, expected, l)

	local, ok := o.Fields[0].Body.(*ast.Local)
	require.True(t, ok)

	nested, ok := local.Body.(*ast.DesugaredObject)
	require.True(t, ok)

	err = om.add(nested, "b")
	require.NoError(t, err)
	l, err = om.lookup(o, []string{"a", "b"})
	require.NoError(t, err)

	expected = jpos.NewLocation("file.jsonnet",
		jpos.NewRangeFromCoords(1, 6, 1, 7))
	assert.Equal(t, expected, l)

}

func Test_pathToLocation(t *testing.T) {
	cases := []struct {
		name         string
		source       string
		pos          jpos.Position
		expectedPath []string
		expectedLoc  jpos.Range
		isErr        bool
	}{
		{
			name:         "position in field name",
			source:       "{a:'a'}",
			pos:          jpos.New(1, 2),
			expectedPath: []string{"a"},
			expectedLoc:  jpos.NewRangeFromCoords(1, 2, 1, 3),
		},
		{
			name:         "position in field body",
			source:       "{a:'a'}",
			pos:          jpos.New(1, 5),
			expectedPath: []string{"a"},
			expectedLoc:  jpos.NewRangeFromCoords(1, 2, 1, 3),
		},
		{
			name:         "position in field body and body is object",
			source:       "{a:{b:'b'}}",
			pos:          jpos.New(1, 5),
			expectedPath: []string{"a", "b"},
			expectedLoc:  jpos.NewRangeFromCoords(1, 5, 1, 6),
		},
		{
			name:         "position in field with string name",
			source:       "{'a': 'a'}",
			pos:          jpos.New(1, 3),
			expectedPath: []string{"a"},
			expectedLoc:  jpos.NewRangeFromCoords(1, 2, 1, 5),
		},
		{
			name:   "position in field with expression name",
			source: "{[a]: 'a'}",
			pos:    jpos.New(1, 3),
			isErr:  true,
		},
		{
			name:         "position in function field parameters",
			source:       "{id(y): y}",
			pos:          jpos.New(1, 5),
			expectedPath: []string{"id"},
			expectedLoc:  jpos.NewRangeFromCoords(1, 5, 1, 6),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node, err := ReadSource("file.jsonnet", tc.source, nil)
			require.NoError(t, err)

			withDesugaredObject(t, node, func(o *ast.DesugaredObject) {
				op, err := pathToLocation(o, tc.pos)
				if tc.isErr {
					require.Error(t, err)
					return
				}

				require.NoError(t, err)
				assert.Equal(t, tc.expectedPath, op.path)
				assert.Equal(t, tc.expectedLoc, op.loc)
			})
		})
	}
}

func withDesugaredObject(t *testing.T, n ast.Node, fn func(o *ast.DesugaredObject)) {
	o, ok := n.(*ast.DesugaredObject)
	require.True(t, ok)

	fn(o)
}
