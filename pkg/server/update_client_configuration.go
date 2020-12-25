package server

import (
	"context"

	"github.com/opentracing/opentracing-go"

	"github.com/tminor/jsonnet-language-server/pkg/config"
	"github.com/tminor/jsonnet-language-server/pkg/lsp"
	"github.com/opentracing/opentracing-go/log"
)

func updateClientConfiguration(ctx context.Context, r *request, c *config.Config) (interface{}, error) {
	span := opentracing.SpanFromContext(ctx)
	ctx = opentracing.ContextWithSpan(ctx, span)
	var update map[string]interface{}
	if err := r.Decode(&update); err != nil {
		return nil, err
	}

	if err := c.UpdateClientConfiguration(ctx, update); err != nil {
		if msgErr := showMessage(ctx, r, lsp.MTError, err.Error()); msgErr != nil {
			span.LogFields(
				log.Error(msgErr),
			)
		}

		return nil, err
	}

	return nil, nil
}
