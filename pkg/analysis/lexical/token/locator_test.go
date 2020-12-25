package token

import (
	"testing"

	jlspos "github.com/tminor/jsonnet-language-server/pkg/util/position"
	"github.com/google/go-jsonnet/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_locator(t *testing.T) {
	cases := []struct {
		name     string
		source   string
		loc      jlspos.Position
		expected ast.LocationRange
	}{
		{
			name:     "locate in missing object body",
			source:   `local a="1";`,
			loc:      jlspos.New(2, 1),
			expected: createRange("file.jsonnet", 1, 13, 1, 13),
		},
		{
			name:     "locate locate body",
			source:   `local a="1";a`,
			loc:      jlspos.New(1, 13),
			expected: createRange("file.jsonnet", 1, 13, 1, 14),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				node, err := Parse("file.jsonnet", tc.source, nil)
				require.NoError(t, err)

				n, err := locateNode(node, tc.loc)
				require.NoError(t, err)

				require.NotNil(t, n.Loc())
				assert.Equal(t, tc.expected.String(), n.Loc().String())
			})
		})
	}
}

func createRange(filename string, r1l, r1c, r2l, r2c int) ast.LocationRange {
	return ast.LocationRange{
		FileName: filename,
		Begin:    createLoc(r1l, r1c),
		End:      createLoc(r2l, r2c),
	}
}
