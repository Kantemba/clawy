// Clawy - Ultra-lightweight personal AI agent
//
// Copyright (c) 2026 Clawy contributors

package telemetry

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	meterName = "github.com/Kantemba/clawy"

	attrsKeyChannel = "clawy.channel"
	attrsKeyStream  = "clawy.stream"
	attrsKeyStatus  = "clawy.status"
)

var (
	messagesReceived metric.Int64Counter
	messagesDropped  metric.Int64Counter
	turnDuration     metric.Float64Histogram
	llmRequests      metric.Int64Counter
)

func initMeters(mp metric.MeterProvider) error {
	meter := mp.Meter(meterName)

	var err error
	if messagesReceived, err = meter.Int64Counter(
		"clawy.messages.received",
		metric.WithDescription("Inbound messages accepted by the gateway bus"),
		metric.WithUnit("{message}"),
	); err != nil {
		return err
	}
	if messagesDropped, err = meter.Int64Counter(
		"clawy.messages.dropped",
		metric.WithDescription("Messages dropped due to backpressure"),
		metric.WithUnit("{message}"),
	); err != nil {
		return err
	}
	if turnDuration, err = meter.Float64Histogram(
		"clawy.agent.turn.duration",
		metric.WithDescription("Duration of full agent turns"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120),
	); err != nil {
		return err
	}
	if llmRequests, err = meter.Int64Counter(
		"clawy.llm.requests",
		metric.WithDescription("LLM completion requests issued to providers"),
		metric.WithUnit("{request}"),
	); err != nil {
		return err
	}
	return nil
}

func counterAdd(counter metric.Int64Counter, value int64, attrs ...attribute.KeyValue) {
	if counter == nil || !Enabled() {
		return
	}
	counter.Add(context.Background(), value, metric.WithAttributes(attrs...))
}

func histogramRecord(histogram metric.Float64Histogram, value float64, attrs ...attribute.KeyValue) {
	if histogram == nil || !Enabled() {
		return
	}
	histogram.Record(context.Background(), value, metric.WithAttributes(attrs...))
}

// RecordMessageReceived counts an inbound message on the given channel.
func RecordMessageReceived(channel string) {
	counterAdd(messagesReceived, 1, attribute.String(attrsKeyChannel, channel))
}

// RecordMessageDropped counts a message dropped by backpressure on a stream.
func RecordMessageDropped(stream string) {
	counterAdd(messagesDropped, 1, attribute.String(attrsKeyStream, stream))
}

// RecordTurnDuration records the wall time and outcome of an agent turn.
func RecordTurnDuration(startedAt time.Time, status string) {
	histogramRecord(turnDuration, time.Since(startedAt).Seconds(),
		attribute.String(attrsKeyStatus, status),
	)
}

// RecordLLMRequest counts one LLM completion request.
func RecordLLMRequest(provider, model string) {
	counterAdd(llmRequests, 1,
		attribute.String("clawy.provider", provider),
		attribute.String("clawy.model", model),
	)
}
