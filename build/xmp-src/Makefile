# =============================================================================
# XMP (Extended Module Player) Static Library Build Makefile
# =============================================================================
# This Makefile builds libxmp as a static library for use with Ikemen GO
# 
# Features:
# - Multi-platform support (Linux, macOS, Windows)
# - Support for multiple module formats (MOD, S3M, XM, IT)
# - Optional module loaders
# 
# Usage:
#   make              Build the static library
#   make clean        Remove built objects and library
#   make install      Install library to output directory
# =============================================================================

# --- Configuration ---
SHELL := /bin/bash
.DEFAULT_GOAL := all

# Auto-detect OS
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
    OS := linux
endif
ifeq ($(UNAME_S),Darwin)
    OS := macos
endif
ifeq ($(OS),)
    OS := windows
endif

# Compiler settings
CC := gcc
AR := ar
RANLIB := ranlib
CFLAGS := -Wall -O2 -fPIC
CFLAGS += -DLIBXMP_STATIC -DLIBXMP_CORE_PLAYER
CFLAGS += -Wno-unused-variable -Wno-unused-parameter

# Platform-specific flags
ifeq ($(OS),windows)
    CFLAGS += -D_WIN32
endif
ifeq ($(OS),macos)
    CFLAGS += -fno-common
endif

# Output
LIB_NAME := libxmp.a
OUTPUT_DIR := ../output
LIB_OUTPUT := $(OUTPUT_DIR)/lib
INCLUDE_OUTPUT := $(OUTPUT_DIR)/include

# Source directories
LOADERS_DIR := loaders
SRC_DIR := .

# Source files
CORE_SOURCES := \
	control.c \
	dataio.c \
	effects.c \
	filetype.c \
	filter.c \
	flow.c \
	format.c \
	hio.c \
	lfo.c \
	load_helpers.c \
	load.c \
	md5.c \
	memio.c \
	misc.c \
	mix_all.c \
	mixer.c \
	period.c \
	player.c \
	read_event.c \
	rng.c \
	scan.c \
	smix.c \
	virtual.c

# Module format loaders
LOADER_SOURCES := \
	loaders/common.c \
	loaders/it_load.c \
	loaders/itsex.c \
	loaders/mod_load.c \
	loaders/s3m_load.c \
	loaders/sample.c \
	loaders/xm_load.c

# Platform-specific sources
ifeq ($(OS),windows)
    CORE_SOURCES += win32.c
endif

# All source files
ALL_SOURCES := $(CORE_SOURCES) $(LOADER_SOURCES)

# Object files
OBJECTS := $(patsubst %.c,%.o,$(ALL_SOURCES))

# Header files to install
HEADERS := xmp.h

# --- Targets ---
.PHONY: all clean install

all: $(LIB_NAME)

$(LIB_NAME): $(OBJECTS)
	@echo "Linking static library: $@"
	@$(AR) rcs $@ $^
	@$(RANLIB) $@
	@echo "Build complete: $@"

%.o: %.c
	@echo "Compiling: $<"
	@$(CC) $(CFLAGS) -c $< -o $@

clean:
	@echo "Cleaning build artifacts..."
	@rm -f $(OBJECTS)
	@rm -f $(LIB_NAME)
	@echo "Clean complete"

install: $(LIB_NAME)
	@echo "Installing library to $(LIB_OUTPUT)..."
	@mkdir -p $(LIB_OUTPUT)
	@mkdir -p $(INCLUDE_OUTPUT)
	@cp $(LIB_NAME) $(LIB_OUTPUT)/
	@for header in $(HEADERS); do \
		cp $$header $(INCLUDE_OUTPUT)/; \
	done
	@echo "Installation complete"

help:
	@echo "XMP Static Library Build Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  all       - Build the static library (default)"
	@echo "  clean     - Remove built objects and library"
	@echo "  install   - Install library to output directory"
	@echo "  help      - Show this help message"
	@echo ""
	@echo "Variables:"
	@echo "  CC        - C compiler (default: gcc)"
	@echo "  CFLAGS    - Compiler flags"
	@echo "  OS        - Operating system (auto-detected)"
	@echo ""
	@echo "Example:"
	@echo "  make CC=clang install"
