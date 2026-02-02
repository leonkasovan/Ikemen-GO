# Static build Makefile for sndcli

CC ?= gcc
PKG_CONFIG ?= pkg-config

TARGET := sndcli
SRC := sndcli.c

CFLAGS ?= -O2 -Wall -Wextra -std=c11

# Detect MinGW and set up paths
ifeq ($(OS),Windows_NT)
    # Windows environment
    MINGW_PREFIX ?= $(shell which gcc | xargs dirname | xargs dirname)
    export PKG_CONFIG_PATH := $(MINGW_PREFIX)/lib/pkgconfig:$(PKG_CONFIG_PATH)
endif

# Try pkg-config for static libsndfile
SND_CFLAGS := $(shell $(PKG_CONFIG) --cflags sndfile 2>/dev/null)
SND_LIBS_STATIC := $(shell $(PKG_CONFIG) --libs --static sndfile 2>/dev/null)

# If pkg-config fails, use fallback hardcoded MinGW paths
ifeq ($(SND_CFLAGS),)
    MINGW_PREFIX ?= $(shell which gcc | xargs dirname | xargs dirname)
    SND_CFLAGS := -I$(MINGW_PREFIX)/include
    SND_LIBS_STATIC := -L$(MINGW_PREFIX)/lib -lsndfile
endif

SND_LIBS := $(if $(SND_LIBS_STATIC),$(SND_LIBS_STATIC),$(shell $(PKG_CONFIG) --libs sndfile 2>/dev/null))

# Common libs often required for static libsndfile builds
EXTRA_LIBS := -lm -lpthread

all: $(TARGET)

$(TARGET): $(SRC)
	$(CC) $(CFLAGS) $(SND_CFLAGS) -o $@ $^ $(SND_LIBS) $(EXTRA_LIBS)

clean:
	rm -f $(TARGET)

.PHONY: all clean
