# Static build Makefile for sndcli

CC ?= gcc
PKG_CONFIG ?= pkg-config

TARGET := sndcli
SRC := sndcli.c

CFLAGS ?= -O2 -Wall -Wextra -std=c11
LDFLAGS ?= -static

# Detect MinGW prefix
MINGW_PREFIX ?= $(shell which gcc | xargs dirname | xargs dirname)

# CFLAGS from pkg-config (for include paths)
SND_CFLAGS := $(shell $(PKG_CONFIG) --cflags sndfile 2>/dev/null)

# Fallback if pkg-config fails
ifeq ($(SND_CFLAGS),)
    SND_CFLAGS := -I$(MINGW_PREFIX)/include -I$(MINGW_PREFIX)/include/opus
endif

# Direct static library linking (true static build)
STATIC_LIBS := \
	-L$(MINGW_PREFIX)/lib \
	-lsndfile \
	-lFLAC \
	-lvorbisenc \
	-lvorbisfile \
	-lvorbis \
	-logg \
	-lopus \
	-lmpg123 \
	-lmp3lame

# System libraries (must use -l flags)
SYS_LIBS := -lm -lssp -lshlwapi

all: $(TARGET)

$(TARGET): $(SRC)
	$(CC) $(CFLAGS) $(LDFLAGS) $(SND_CFLAGS) -o $@ $^ $(STATIC_LIBS) $(SYS_LIBS)

clean:
	rm -f $(TARGET) $(TARGET).exe

install: all
	cp $(TARGET) D:/w64devkit/bin

.PHONY: all clean
