/*
MIT License

Copyright (c) 2022 Kazumasa Kohtaka

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"

	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	tracingLog = ctrl.Log.WithName("tracing")

	providerResource *resource.Resource = resource.Default()
)

func init() {
	if r, err := resource.Merge(
		providerResource,
		resource.NewSchemaless(
			attribute.String(string(semconv.ServiceNameKey), "kubernetesimal"),
			// TODO: Set the version of the service
		),
	); err != nil {
		tracingLog.Error(err, "unable to merge TraceProviderResources")
	} else {
		providerResource = r
	}
}

func NewTracerProvider(
	ctx context.Context,
	httpAddr, grpcAddr string,
) (*tracesdk.TracerProvider, error) {
	traceCtx := context.Background()

	var opts []tracesdk.TracerProviderOption
	if httpAddr != "" {
		exporter, err := otlptrace.New(
			traceCtx,
			otlptracehttp.NewClient(
				otlptracehttp.WithEndpoint(httpAddr),
				otlptracehttp.WithInsecure(),
			),
		)
		if err != nil {
			return nil, fmt.Errorf("unable to start OTLP exporter: %w", err)
		}
		opts = append(opts, tracesdk.WithBatcher(exporter))
	}
	if grpcAddr != "" {
		exporter, err := otlptrace.New(
			traceCtx,
			otlptracegrpc.NewClient(
				otlptracegrpc.WithEndpoint(grpcAddr),
				otlptracegrpc.WithInsecure(),
			),
		)
		if err != nil {
			return nil, fmt.Errorf("unable to start OTLP exporter: %w", err)
		}
		opts = append(opts, tracesdk.WithBatcher(exporter))
	}

	opts = append(opts, tracesdk.WithResource(providerResource))

	provider := tracesdk.NewTracerProvider(opts...)

	go func() {
		<-ctx.Done()

		if err := provider.Shutdown(traceCtx); err != nil {
			tracingLog.Error(err, "unable to shutdown OTLP provider")
		}
	}()

	return provider, nil
}

type contextKey struct{}

// FromContext returns a tracer with predefined values from a context.Context.
func FromContext(ctx context.Context) trace.Tracer {
	if v, ok := ctx.Value(contextKey{}).(trace.Tracer); ok {
		return v
	}
	return otel.GetTracerProvider().Tracer("")
}

// NewContext returns a new context derived from ctx that embeds the tracer.
func NewContext(ctx context.Context, tracer trace.Tracer) context.Context {
	return context.WithValue(ctx, contextKey{}, tracer)
}
