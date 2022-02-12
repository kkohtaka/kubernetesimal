module github.com/kkohtaka/kubernetesimal

go 1.16

require (
	github.com/emicklei/go-restful v2.10.0+incompatible // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/go-cmp v0.5.7 // indirect
	github.com/onsi/ginkgo v1.14.1
	github.com/onsi/gomega v1.10.2
	github.com/stretchr/testify v1.7.0
	go.opentelemetry.io/otel v1.2.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.2.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.2.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.2.0
	go.opentelemetry.io/otel/sdk v1.2.0
	go.opentelemetry.io/proto/otlp v0.12.0 // indirect
	golang.org/x/crypto v0.0.0-20210921155107-089bfa567519
	k8s.io/api v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.20.2
	kubevirt.io/api v0.49.0
	sigs.k8s.io/controller-runtime v0.8.3
)
