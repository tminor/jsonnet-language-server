package token

import (
	"testing"

	jlspos "github.com/tminor/jsonnet-language-server/pkg/util/position"
	"github.com/google/go-jsonnet/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScope(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		loc      jlspos.Position
		expected []string
		isErr    bool
	}{
		{
			name:     "valid local",
			src:      `local a="a";a`,
			loc:      jlspos.New(1, 13),
			expected: []string{"a", "std"},
		},
		{
			name:     "local with no body",
			src:      `local a="a";`,
			loc:      jlspos.New(2, 1),
			expected: []string{"a", "std"},
		},
		{
			name:     "local with an incomplete body",
			src:      "local o={a:'b'};\nlocal y=o.\n",
			loc:      jlspos.New(2, 11),
			expected: []string{"o", "std", "y"},
		},
		{
			name:     "object keys in invalid local",
			src:      `local o={a:"a"};`,
			loc:      jlspos.New(2, 1),
			expected: []string{"o", "std"},
		},
		{
			name:     "deep object",
			src:      `local o={a:{b:{c:{d:"e"}}}};o.a.b.c.d.e`,
			loc:      jlspos.New(1, 36),
			expected: []string{"o", "std"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nc := NewNodeCache()
			sm, err := LocationScope("file.jsonnet", tc.src, tc.loc, nc)
			if tc.isErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expected, sm.Keys())
		})
	}
}

func TestScopeMap(t *testing.T) {
	nc := NewNodeCache()
	sm := newScope(nc)
	o := &ast.Object{}
	sm.add(ast.Identifier("foo"), o)

	expectedKeys := []string{"foo"}
	require.Equal(t, expectedKeys, sm.Keys())

	expectedEntry := &ScopeEntry{
		Detail: "foo",
		Node:   o,
	}

	e, err := sm.Get("foo")
	require.NoError(t, err)

	require.Equal(t, expectedEntry, e)
}

func TestScopeMap_Get_invalid(t *testing.T) {
	nc := NewNodeCache()

	sm := newScope(nc)
	_, err := sm.Get("invalid")
	require.Error(t, err)
}

func TestScope_GetPath(t *testing.T) {
	b := &ast.DesugaredObject{
		Fields: ast.DesugaredObjectFields{
			{
				Name: createLiteralString("c"),
				Body: &ast.Local{
					Body: createLiteralString("a"),
				},
			},
		},
	}

	data := &ast.DesugaredObject{
		Fields: ast.DesugaredObjectFields{
			{
				Name: createLiteralString("a"),
				Body: &ast.Local{
					Body: createLiteralString("a"),
				},
			},
			{
				Name: createLiteralString("b"),
				Body: &ast.Local{
					Body: b,
				},
			},
		},
	}

	o := &ast.DesugaredObject{
		Fields: ast.DesugaredObjectFields{
			{
				Name: createLiteralString("data"),
				Body: &ast.Local{
					Body: data,
				},
			},
			{
				Name: createLiteralString("str"),
				Body: &ast.Local{
					Body: createLiteralString("str"),
				},
			},
		},
	}

	nc := NewNodeCache()

	s := newScope(nc)
	s.add(createIdentifier("o"), o)

	cases := []struct {
		name     string
		path     []string
		expected *ScopeEntry
		isErr    bool
	}{
		{
			name: "at root",
			path: []string{"o"},
			expected: &ScopeEntry{
				Detail: "o",
				Node:   o,
			},
		},
		{
			name: "nested",
			path: []string{"o", "data"},
			expected: &ScopeEntry{
				Detail: "(object) {\n  (field) a::,\n  (field) b::,\n}",
				Node:   data,
			},
		},
		{
			name: "nested 2",
			path: []string{"o", "data", "b"},
			expected: &ScopeEntry{
				Detail: "(object) {\n  (field) c::,\n}",
				Node:   b,
			},
		},
		{
			name:  "invalid path",
			path:  []string{"x"},
			isErr: true,
		},
		{
			name:  "invalid nested path",
			path:  []string{"o", "x"},
			isErr: true,
		},
		{
			name:  "item not an object",
			path:  []string{"o", "str", "x"},
			isErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.GetInPath(tc.path)
			if !tc.isErr && assert.NoError(t, err) {
				assert.Equal(t, tc.expected, got)
				return
			}

			require.Error(t, err)
		})
	}
}

func createLiteralString(v string) *ast.LiteralString {
	return &ast.LiteralString{
		Value: v,
		Kind:  ast.StringSingle,
	}
}
