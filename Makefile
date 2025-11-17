# ------------------------------
# Build configuration
# ------------------------------

GO       := go
BUILD_DATE := $(shell date +%Y%m%d_%H%M%S 2>/dev/null || echo unknown_date)
BIN_DIR  := bin
SRC_DIR  := src
ASSETS   := $(SRC_DIR)/assets.zip
SCREENPACK := $(SRC_DIR)/screenpack.zip

# ------------------------------
# Detect platform
# ------------------------------

UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)

ifeq ($(UNAME_S),Linux)
    DEFAULT_TARGET := linux
	ifeq ($(UNAME_M),aarch64)
# 		TAGS := sdl2,opengles31,gles2
		TAGS := x11,opengles31,gles2
# 		TAGS := x11,vulkan
# 		TAGS := wayland,opengles31,gles2
# 		TAGS := opengl21
	else
		TAGS := opengl33
	endif
else ifeq ($(OS),Windows_NT)
    DEFAULT_TARGET := win
	TAGS := opengl33
else
    DEFAULT_TARGET := linux
	TAGS := opengl33
endif

# ------------------------------
# Main targets
# ------------------------------

.PHONY: all win linux clean assets screenpack

all: $(DEFAULT_TARGET)

win:
	@echo "Building for Windows with $(TAGS)..."
	CGO_ENABLED=1 GOEXPERIMENT=arenas GOOS=windows GOARCH=amd64 \
	$(GO) build -tags=$(TAGS) -trimpath -v -ldflags "-s -w -H windowsgui" \
	-o $(BIN_DIR)/ikemen_win.exe ./$(SRC_DIR)

linux: $(ASSETS) $(SCREENPACK)
	@echo "Building for Linux with $(TAGS)..."
	CGO_ENABLED=1 GOEXPERIMENT=arenas GOOS=linux \
	$(GO) build -x -tags=$(TAGS) -trimpath -v -ldflags "-s -w" \
	-o $(BIN_DIR)/ikemen_linux ./$(SRC_DIR)

# ------------------------------
# Asset packaging
# ------------------------------

$(ASSETS): data/* external/* font/*
	@echo "Packaging assets..."
	echo $(BUILD_DATE) > external/script/version
	rm -f $(ASSETS)
	cd $(SRC_DIR) && zip -r assets.zip ../data ../external ../font >/dev/null

$(SCREENPACK):
	@echo "Downloading screenpack..."
#	wget -nc -P $(SRC_DIR) https://github.com/leonkasovan/Ikemen-GO/releases/download/v1.0/screenpack.zip

# ------------------------------
# Utility targets
# ------------------------------

clean:
	@echo "Cleaning up..."
	rm -f $(ASSETS)
	rm -rf $(BIN_DIR)/*
