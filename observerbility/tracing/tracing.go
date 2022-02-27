package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"

	ctrl "sigs.k8s.io/controller-runtime"
	//+kubebuilder:scaffold:imports
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
) (*trace.TracerProvider, error) {
	traceCtx := context.Background()

	var opts []trace.TracerProviderOption
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
		opts = append(opts, trace.WithBatcher(exporter))
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
		opts = append(opts, trace.WithBatcher(exporter))
	}

	opts = append(opts, trace.WithResource(providerResource))

	provider := trace.NewTracerProvider(opts...)

	go func() {
		<-ctx.Done()

		if err := provider.Shutdown(traceCtx); err != nil {
			tracingLog.Error(err, "unable to shutdown OTLP provider")
		}
	}()

	return provider, nil
}
