# Code Generator Binaries

CODE_GENERATORS= \
	bin/defaulter-gen \
	bin/client-gen \
	bin/lister-gen \
	bin/informer-gen \
	bin/deepcopy-gen

$(CODE_GENERATORS):
	go build -o bin/defaulter-gen vendor/k8s.io/code-generator/cmd/defaulter-gen/main.go
	go build -o bin/client-gen    vendor/k8s.io/code-generator/cmd/client-gen/main.go
	go build -o bin/lister-gen    vendor/k8s.io/code-generator/cmd/lister-gen/main.go
	go build -o bin/informer-gen  vendor/k8s.io/code-generator/cmd/informer-gen/main.go
	go build -o bin/deepcopy-gen  vendor/k8s.io/code-generator/cmd/deepcopy-gen/main.go


# Code Generation

OUTPUT_PKG=github.com/kkohtaka/kubernetesimal/pkg/client
APIS_PKG=github.com/kkohtaka/kubernetesimal/pkg/apis
APIS=github/v1alpha1
FQ_APIS=github.com/kkohtaka/kubernetesimal/pkg/apis/github/v1alpha1

OUTPUT_FILE_BASE=zz_generated
CLIENTSET_NAME=versioned
GO_HEADER_FILE=./hack/boilerplate.go.txt

.PHONY: deepcopy-gen
deepcopy-gen: $(CODE_GENERATORS)
	./bin/deepcopy-gen \
		--go-header-file $(GO_HEADER_FILE) \
		--input-dirs $(FQ_APIS) \
		--output-file-base $(OUTPUT_FILE_BASE).deepcopy \
		--bounding-dirs $(APIS_PKG)

.PHONY: defaulter-gen
defaulter-gen: $(CODE_GENERATORS)
	./bin/defaulter-gen \
		--go-header-file $(GO_HEADER_FILE) \
		--input-dirs $(FQ_APIS) \
		--output-file-base $(OUTPUT_FILE_BASE).defaults

.PHONY: client-gen
client-gen: $(CODE_GENERATORS)
	./bin/client-gen \
		--go-header-file $(GO_HEADER_FILE) \
		--input $(APIS) \
		--input-base $(APIS_PKG) \
		--input-dirs $(FQ_APIS) \
		--clientset-name $(CLIENTSET_NAME) \
		--output-package $(OUTPUT_PKG)/clientset

.PHONY: lister-gen
lister-gen: $(CODE_GENERATORS)
	./bin/lister-gen \
		--go-header-file $(GO_HEADER_FILE) \
		--input-dirs $(FQ_APIS) \
		--output-package $(OUTPUT_PKG)/listers

.PHONY: informer-gen
informer-gen: $(CODE_GENERATORS)
	./bin/informer-gen \
		--go-header-file $(GO_HEADER_FILE) \
		--input-dirs $(FQ_APIS) \
		--versioned-clientset-package $(OUTPUT_PKG)/clientset/$(CLIENTSET_NAME) \
		--listers-package $(OUTPUT_PKG)/listers \
		--output-package $(OUTPUT_PKG)/informers

.PHONY: codegen
codegen: deepcopy-gen defaulter-gen client-gen lister-gen informer-gen
