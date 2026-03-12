package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Common attribute keys for sandbox operations.
const (
	AttrSandboxName   = "sandbox.name"
	AttrSandboxState  = "sandbox.state"
	AttrMatrixName    = "matrix.name"
	AttrSessionID     = "session.id"
	AttrBlueprintName = "blueprint.name"
	AttrTeamName      = "team.name"
	AttrUserName      = "user.name"
	AttrExecCommand   = "exec.command"
)

// Tracer returns a named tracer for the given component.
func Tracer(component string) trace.Tracer {
	return otel.Tracer("sandboxmatrix/" + component)
}

// StartSpan starts a new span with standard attributes. The returned context
// carries the new span and must be used for downstream calls so that child
// spans are linked correctly. Callers must call span.End() when done (usually
// via defer).
func StartSpan(ctx context.Context, component, operation string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	ctx, span := Tracer(component).Start(ctx, component+"."+operation,
		trace.WithAttributes(attrs...),
	)
	return ctx, span
}

// RecordError records an error on the span in the current context. If there
// is no active span this is a no-op.
func RecordError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// SetSpanAttributes adds attributes to the span in the current context.
func SetSpanAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attrs...)
}
