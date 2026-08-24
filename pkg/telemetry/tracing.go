// Clawy - Ultra-lightweight personal AI agent
//
// Copyright (c) 2026 Clawy contributors

package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Tracer returns the shared application tracer. Before Setup runs this is a
// no-op tracer; afterwards it delegates to the configured provider.
func Tracer() trace.Tracer {
	return otel.Tracer(meterName)
}

// StartTurnSpan starts the root-ish span for one inbound agent turn.
func StartTurnSpan(ctx context.Context, channel, chatID, senderID, sessionKey string, mediaCount int) (context.Context, trace.Span) {
	return Tracer().Start(ctx, "agent.turn",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String(attrsKeyChannel, channel),
			attribute.String("clawy.chat_id", chatID),
			attribute.String("clawy.sender_id", senderID),
			attribute.String("clawy.session_key", sessionKey),
			attribute.Int("clawy.media_count", mediaCount),
		),
	)
}

// FinishSpan records the outcome on a span and ends it.
func FinishSpan(span trace.Span, err error) {
	if span == nil {
		return
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
}
