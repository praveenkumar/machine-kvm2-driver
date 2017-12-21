# kvm2 Plugin versions
PLUGIN_VERSION = 0.11.0

# Go and compilation related variables
BUILD_DIR ?= out

ORG := github.com/praveenkumar
REPOPATH ?= $(ORG)/machine-kvm2-driver

# Linker flags
VERSION_VARIABLES := -X $(REPOPATH)/main.pluginVersion=$(PLUGIN_VERSION) 

LDFLAGS := $(VERSION_VARIABLES)

vendor:
	dep ensure -v

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)
	rm -rf vendor

.PHONY: build
build: $(BUILD_DIR) vendor
	go build \
			-installsuffix "static" \
			-ldflags "$(LDFLAGS)" \
			-tags libvirt.1.3.1 \
			-o $(BUILD_DIR)/docker-machine-driver-kvm2