/*
  SDL2 Gamepad Mapper.
  Copyright (C) 2025 Dhani Novan (dhani.novan@gmail.com)
  Build in Ubuntu 20.0.4 with SDL2 2.0.10.

  Usage:
	# Generate default gamecontrollerdb.txt in current directory
	sdlGamepadMapper

	# Print connected joystick GUID and exit
	sdlGamepadMapper --guid

	# Generate db mapper in external/mycustomdb.txt
	sdlGamepadMapper external/mycustomdb.txt

	# Copy entry for connected joystick from source/gamecontrollerdb.txt into dest/gamecontrollerdb.txt
	sdlGamepadMapper --guid source/gamecontrollerdb.txt dest/gamecontrollerdb.txt
*/

#include <stdio.h>
#include <unistd.h>
#include <SDL2/SDL.h>

static const unsigned char embedded_font[] = { 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x30, 0x30, 0x30, 0x00, 0x30, 0x00, 0x00, 0x00, 0x50, 0x50, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x50, 0xf8, 0x50, 0xf8, 0x50, 0x00, 0x00, 0x00, 0x78, 0xa0, 0x70, 0x28, 0xf0, 0x00, 0x00, 0x00, 0x88, 0x10, 0x20, 0x40, 0x88, 0x00, 0x00, 0x00, 0x40, 0xa0, 0x68, 0x90, 0x68, 0x00, 0x00, 0x00, 0x20, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x20, 0x40, 0x40, 0x40, 0x20, 0x00, 0x00, 0x00, 0x40, 0x20, 0x20, 0x20, 0x40, 0x00, 0x00, 0x00, 0x20, 0xa8, 0x70, 0xa8, 0x20, 0x00, 0x00, 0x00, 0x00, 0x20, 0x70, 0x20, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x60, 0x20, 0x40, 0x00, 0x00, 0x00, 0x00, 0x70, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x60, 0x60, 0x00, 0x00, 0x00, 0x08, 0x10, 0x20, 0x40, 0x80, 0x00, 0x00, 0x00, 0x70, 0xc8, 0xc8, 0xc8, 0x70, 0x00, 0x00, 0x00, 0x30, 0x70, 0x30, 0x30, 0x78, 0x00, 0x00, 0x00, 0xf0, 0x18, 0x70, 0xc0, 0xf8, 0x00, 0x00, 0x00, 0xf8, 0x18, 0x30, 0x98, 0x70, 0x00, 0x00, 0x00, 0x30, 0x70, 0xd0, 0xf8, 0x10, 0x00, 0x00, 0x00, 0xf8, 0xc0, 0xf0, 0x18, 0xf0, 0x00, 0x00, 0x00, 0x70, 0xc0, 0xf0, 0xc8, 0x70, 0x00, 0x00, 0x00, 0xf8, 0x18, 0x30, 0x60, 0xc0, 0x00, 0x00, 0x00, 0x70, 0xc8, 0x70, 0xc8, 0x70, 0x00, 0x00, 0x00, 0x70, 0xc8, 0x78, 0x08, 0x70, 0x00, 0x00, 0x00, 0x60, 0x60, 0x00, 0x60, 0x60, 0x00, 0x00, 0x00, 0x60, 0x60, 0x00, 0x60, 0x20, 0x40, 0x00, 0x00, 0x10, 0x20, 0x40, 0x20, 0x10, 0x00, 0x00, 0x00, 0x00, 0x70, 0x00, 0x70, 0x00, 0x00, 0x00, 0x00, 0x40, 0x20, 0x10, 0x20, 0x40, 0x00, 0x00, 0x00, 0x78, 0x18, 0x30, 0x00, 0x30, 0x00, 0x00, 0x00, 0x70, 0xa8, 0xb8, 0x80, 0x70, 0x00, 0x00, 0x00, 0x70, 0xc8, 0xc8, 0xf8, 0xc8, 0x00, 0x00, 0x00, 0xf0, 0xc8, 0xf0, 0xc8, 0xf0, 0x00, 0x00, 0x00, 0x70, 0xc8, 0xc0, 0xc8, 0x70, 0x00, 0x00, 0x00, 0xf0, 0xc8, 0xc8, 0xc8, 0xf0, 0x00, 0x00, 0x00, 0xf8, 0xc0, 0xf0, 0xc0, 0xf8, 0x00, 0x00, 0x00, 0xf8, 0xc0, 0xf0, 0xc0, 0xc0, 0x00, 0x00, 0x00, 0x78, 0xc0, 0xd8, 0xc8, 0x78, 0x00, 0x00, 0x00, 0xc8, 0xc8, 0xf8, 0xc8, 0xc8, 0x00, 0x00, 0x00, 0x78, 0x30, 0x30, 0x30, 0x78, 0x00, 0x00, 0x00, 0xf8, 0x18, 0x18, 0xd8, 0x70, 0x00, 0x00, 0x00, 0xc8, 0xd0, 0xe0, 0xd0, 0xc8, 0x00, 0x00, 0x00, 0xc0, 0xc0, 0xc0, 0xc0, 0xf8, 0x00, 0x00, 0x00, 0xd8, 0xf8, 0xf8, 0xa8, 0x88, 0x00, 0x00, 0x00, 0xc8, 0xe8, 0xf8, 0xd8, 0xc8, 0x00, 0x00, 0x00, 0x70, 0xc8, 0xc8, 0xc8, 0x70, 0x00, 0x00, 0x00, 0xf0, 0xc8, 0xc8, 0xf0, 0xc0, 0x00, 0x00, 0x00, 0x70, 0xc8, 0xc8, 0xc8, 0x70, 0x08, 0x00, 0x00, 0xf0, 0xc8, 0xc8, 0xf0, 0xc8, 0x00, 0x00, 0x00, 0x78, 0xe0, 0x70, 0x38, 0xf0, 0x00, 0x00, 0x00, 0x78, 0x30, 0x30, 0x30, 0x30, 0x00, 0x00, 0x00, 0xc8, 0xc8, 0xc8, 0xc8, 0x70, 0x00, 0x00, 0x00, 0xc8, 0xc8, 0xc8, 0x70, 0x20, 0x00, 0x00, 0x00, 0x88, 0xa8, 0xf8, 0xf8, 0xd8, 0x00, 0x00, 0x00, 0xc8, 0xc8, 0x70, 0xc8, 0xc8, 0x00, 0x00, 0x00, 0x68, 0x68, 0x78, 0x30, 0x30, 0x00, 0x00, 0x00, 0xf8, 0x30, 0x60, 0xc0, 0xf8, 0x00, 0x00, 0x00, 0x60, 0x40, 0x40, 0x40, 0x60, 0x00, 0x00, 0x00, 0x80, 0x40, 0x20, 0x10, 0x08, 0x00, 0x00, 0x00, 0x60, 0x20, 0x20, 0x20, 0x60, 0x00, 0x00, 0x00, 0x20, 0x50, 0x88, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x78, 0x00, 0x00, 0x00, 0x40, 0x20, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x78, 0x98, 0x98, 0x78, 0x00, 0x00, 0x00, 0xc0, 0xf0, 0xc8, 0xc8, 0xf0, 0x00, 0x00, 0x00, 0x00, 0x78, 0xe0, 0xe0, 0x78, 0x00, 0x00, 0x00, 0x18, 0x78, 0x98, 0x98, 0x78, 0x00, 0x00, 0x00, 0x00, 0x70, 0xd8, 0xe0, 0x70, 0x00, 0x00, 0x00, 0x38, 0x60, 0xf8, 0x60, 0x60, 0x00, 0x00, 0x00, 0x00, 0x70, 0x98, 0xf8, 0x18, 0x70, 0x00, 0x00, 0xc0, 0xf0, 0xc8, 0xc8, 0xc8, 0x00, 0x00, 0x00, 0x30, 0x00, 0x70, 0x30, 0x78, 0x00, 0x00, 0x00, 0x18, 0x00, 0x18, 0x18, 0x98, 0x70, 0x00, 0x00, 0xc0, 0xc8, 0xf0, 0xc8, 0xc8, 0x00, 0x00, 0x00, 0x60, 0x60, 0x60, 0x60, 0x38, 0x00, 0x00, 0x00, 0x00, 0xd0, 0xf8, 0xa8, 0xa8, 0x00, 0x00, 0x00, 0x00, 0xf0, 0xc8, 0xc8, 0xc8, 0x00, 0x00, 0x00, 0x00, 0x70, 0xc8, 0xc8, 0x70, 0x00, 0x00, 0x00, 0x00, 0xf0, 0xc8, 0xc8, 0xf0, 0xc0, 0x00, 0x00, 0x00, 0x78, 0x98, 0x98, 0x78, 0x18, 0x00, 0x00, 0x00, 0xf0, 0xc8, 0xc0, 0xc0, 0x00, 0x00, 0x00, 0x00, 0x78, 0xe0, 0x38, 0xf0, 0x00, 0x00, 0x00, 0x60, 0xf8, 0x60, 0x60, 0x38, 0x00, 0x00, 0x00, 0x00, 0x98, 0x98, 0x98, 0x78, 0x00, 0x00, 0x00, 0x00, 0xc8, 0xc8, 0xd0, 0xe0, 0x00, 0x00, 0x00, 0x00, 0x88, 0xa8, 0xf8, 0xd8, 0x00, 0x00, 0x00, 0x00, 0xd8, 0x70, 0x70, 0xd8, 0x00, 0x00, 0x00, 0x00, 0x98, 0x98, 0x78, 0x18, 0x70, 0x00, 0x00, 0x00, 0xf8, 0x30, 0x60, 0xf8, 0x00, 0x00, 0x00, 0x30, 0x20, 0x60, 0x20, 0x30, 0x00, 0x00, 0x00, 0x20, 0x20, 0x20, 0x20, 0x20, 0x00, 0x00, 0x00, 0x60, 0x20, 0x30, 0x20, 0x60, 0x00, 0x00, 0x00, 0x00, 0x28, 0x50, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 };
#define FONT_CHAR_COUNT 128
#define FONT_HEIGHT 8
#define FONT_WIDTH 6

typedef struct {
	SDL_Texture* atlas;
	SDL_Rect rects[FONT_CHAR_COUNT];
	int atlas_width;
	int atlas_height;
	int loaded;
} FontCache;

#define AXIS_DEADZONE 16000
#define INPUT_TIMEOUT 6000

typedef enum { MAP_BUTTON, MAP_AXIS, MAP_HAT } MapType;

typedef struct {
	const char* logical;   // SDL logical name (e.g., "a", "leftx", "dpup")
	const char* prompt;    // What we show to the user
	int is_axis;           // For convenience in prompts
} LogicalInput;

// Order loosely follows common expectations
static const LogicalInput kOrder[] = {
	{"a", "Press A (bottom face button)", 0},
	{"b", "Press B (right face button)", 0},
	{"x", "Press X (left face button)", 0},
	{"y", "Press Y (top face button)", 0},
	{"back", "Press Back/Select", 0},
	{"guide", "Press Guide/Home", 0},
	{"start", "Press Start/Options", 0},
	{"leftstick", "Click Left Stick (press it down)", 0},
	{"rightstick", "Click Right Stick (press it down)", 0},
	{"leftshoulder", "Press Left Shoulder (L1/LB)", 0},
	{"rightshoulder", "Press Right Shoulder (R1/RB)", 0},
	{"dpup", "Press D-Pad UP", 0},
	{"dpdown", "Press D-Pad DOWN", 0},
	{"dpleft", "Press D-Pad LEFT", 0},
	{"dpright", "Press D-Pad RIGHT", 0},
	{"leftx", "Move LEFT STICK horizontally (push strongly RIGHT)", 1},
	{"lefty", "Move LEFT STICK vertically (push strongly UP)", 1},
	{"rightx", "Move RIGHT STICK horizontally (push strongly RIGHT)", 1},
	{"righty", "Move RIGHT STICK vertically (push strongly UP)", 1},
	{"lefttrigger", "Press LEFT Trigger (L2/LT)", 1},
	{"righttrigger", "Press RIGHT Trigger (R2/RT)", 1},
};

typedef struct {
	int assigned;             // 0/1
	MapType type;             // button/axis/hat
	int index;                // button index / axis index / hat index
	int direction;            // for hat: SDL_HAT_* mask; for axis: sign (-1 or +1); unused for button
} MappingEntry;

FontCache font_cache = { 0 };

static inline void append(char** ptr, char* end, const char* fmt, ...) {
	if (*ptr >= end) return;

	va_list args;
	va_start(args, fmt);
	int n = vsnprintf(*ptr, end - *ptr, fmt, args);
	va_end(args);

	if (n > 0) {
		*ptr += (n < (end - *ptr)) ? n : (end - *ptr); // advance but don't go past end
	}

	// output to stdout as well
	// printf(fmt, args);
}

void init_font_cache(SDL_Renderer* renderer) {
	if (font_cache.loaded) return;

	// Estimate atlas size (simple horizontal layout)
	int padding = 1;
	int total_width = 0;
	for (int i = 0; i < FONT_CHAR_COUNT; ++i)
		total_width += FONT_WIDTH + padding;
	font_cache.atlas_width = total_width;
	font_cache.atlas_height = FONT_HEIGHT;

	SDL_Surface* atlas_surface = SDL_CreateRGBSurfaceWithFormat(0, font_cache.atlas_width, FONT_HEIGHT, 32, SDL_PIXELFORMAT_RGBA8888);
	if (!atlas_surface) return;

	// Fill transparent
	SDL_FillRect(atlas_surface, NULL, SDL_MapRGBA(atlas_surface->format, 0, 0, 0, 0));

	int x_offset = 0;
	for (int i = 0; i < FONT_CHAR_COUNT; ++i) {
		int width = FONT_WIDTH;
		if (width == 0) {
			font_cache.rects[i] = (SDL_Rect){ 0, 0, 0, 0 };
			continue;
		}

		for (int row = 0; row < FONT_HEIGHT; ++row) {
			unsigned char bits = embedded_font[i * FONT_HEIGHT + row];
			for (int col = 8 - width, x = 0; col < 8; ++col, ++x) {
				if (bits & (1 << col)) {
					Uint32* pixels = (Uint32*) atlas_surface->pixels;
					pixels[row * atlas_surface->w + (x_offset + x)] = SDL_MapRGBA(atlas_surface->format, 255, 255, 255, 255);
				}
			}
		}

		font_cache.rects[i] = (SDL_Rect){ x_offset, 0, width, FONT_HEIGHT };
		x_offset += width + padding;
	}

	font_cache.atlas = SDL_CreateTextureFromSurface(renderer, atlas_surface);
	SDL_SetTextureBlendMode(font_cache.atlas, SDL_BLENDMODE_BLEND);
	SDL_FreeSurface(atlas_surface);
	font_cache.loaded = 1;
}

void free_font_cache() {
	if (font_cache.atlas) {
		SDL_DestroyTexture(font_cache.atlas);
		font_cache.atlas = NULL;
	}
	font_cache.loaded = 0;
}

// Draw a single character on the screen using an embedded 5x7 font.
void draw_char(SDL_Renderer* renderer, unsigned char symbol, int x, int y, SDL_Color color) {
	if (!font_cache.loaded || !font_cache.atlas) return;

	int flip_vertically = 0;
	if (symbol > 127) {
		flip_vertically = 1;
		symbol -= 128;
	}

	SDL_Rect src = font_cache.rects[symbol];
	if (src.w == 0 || src.h == 0) return;

	SDL_Rect dst = { x, y, src.w, src.h };

	SDL_SetTextureColorMod(font_cache.atlas, color.r, color.g, color.b);
	SDL_SetTextureAlphaMod(font_cache.atlas, color.a);

	SDL_RendererFlip flip = flip_vertically ? SDL_FLIP_VERTICAL : SDL_FLIP_NONE;
	SDL_RenderCopyEx(renderer, font_cache.atlas, &src, &dst, 0.0, NULL, SDL_FLIP_HORIZONTAL | flip);
}

// Draw scaled a single character
void draw_char_scaled(SDL_Renderer* renderer, unsigned char symbol, int x, int y, int scale, SDL_Color color) {
	if (!font_cache.loaded || !font_cache.atlas) return;

	int flip_vertically = 0;
	if (symbol > 127) {
		flip_vertically = 1;
		symbol -= 128;
	}

	SDL_Rect src = font_cache.rects[symbol];
	if (src.w == 0 || src.h == 0) return;

	SDL_Rect dst = { x, y, src.w * scale, src.h * scale };

	SDL_SetTextureColorMod(font_cache.atlas, color.r, color.g, color.b);
	SDL_SetTextureAlphaMod(font_cache.atlas, color.a);

	SDL_RendererFlip flip = flip_vertically ? SDL_FLIP_VERTICAL : SDL_FLIP_NONE;
	SDL_RenderCopyEx(renderer, font_cache.atlas, &src, &dst, 0.0, NULL, SDL_FLIP_HORIZONTAL | flip);
}

void draw_string(SDL_Renderer* renderer, const char* text, int orig_x, int orig_y, SDL_Color color) {
	int x = orig_x, y = orig_y;

	for (const char* p = text; *p; ++p) {
		if (*p == '\n') {
			x = orig_x;
			y += FONT_HEIGHT;
		} else {
			draw_char(renderer, (*p), x, y, color);
			x += FONT_WIDTH;
		}
	}
}

void draw_string_scaled(SDL_Renderer* renderer, const char* text, int orig_x, int orig_y, int scale, SDL_Color color) {
	int x = orig_x, y = orig_y;

	for (const char* p = text; *p; ++p) {
		if (*p == '\n') {
			x = orig_x;
			y += FONT_HEIGHT * scale;
		} else {
			draw_char_scaled(renderer, (*p), x, y, scale, color);
			x += FONT_WIDTH * scale;
		}
	}
}

static int wait_for_release() {
	// Briefly wait until things settle (no big axis motion / button down)
	Uint32 start = SDL_GetTicks();
	while (SDL_GetTicks() - start < 250) {
		SDL_Event e;
		while (SDL_PollEvent(&e)) {
			if (e.type == SDL_QUIT) return 0;
		}
		SDL_Delay(5);
	}
	return 1;
}

static void token_from_entry(const MappingEntry* e, char* buf, size_t bufsz) {
	switch (e->type) {
	case MAP_BUTTON:
		SDL_snprintf(buf, bufsz, "b%d", e->index);
		break;
	case MAP_AXIS:
		SDL_snprintf(buf, bufsz, "a%d", e->index);
		break;
	case MAP_HAT:
		SDL_snprintf(buf, bufsz, "h%d.%d", e->index, e->direction);
		break;
	}
}

#define WINDOW_WIDTH 640
#define WINDOW_HEIGHT 480
#define MAX_JOYSTICK 4

int main(int argc, char* argv[]) {
	char cwd[1024];
	if (getcwd(cwd, sizeof(cwd)) != NULL) {
		printf("[sdlGamepadMapper.c] Current working dir: %s\n", cwd);
	}

	if (SDL_Init(SDL_INIT_VIDEO | SDL_INIT_AUDIO | SDL_INIT_JOYSTICK | SDL_INIT_GAMECONTROLLER) != 0) {
		printf("SDL_Init Error: %s\n", SDL_GetError());
		return 1;
	}

	if (argc > 1) {
		char guid[MAX_JOYSTICK][64];
		int numJoysticks = SDL_NumJoysticks();

		if (strcmp(argv[1], "--guid") == 0) {
			if (numJoysticks > MAX_JOYSTICK)
				numJoysticks = MAX_JOYSTICK;
			for (int i = 0; i < numJoysticks; i++) {
				SDL_Joystick* joy = SDL_JoystickOpen(i);
				if (joy) {
					SDL_JoystickGetGUIDString(SDL_JoystickGetGUID(joy), guid[i], sizeof(guid[i]));
					if (argc == 2) printf("%s ", guid[i]);
					SDL_JoystickClose(joy);
				}
			}
		}

		if ((argc == 3 || argc == 4) && strcmp(argv[1], "--guid") == 0) {
			FILE* fo;
			// Open the gamecontrollerdb.txt file
			FILE* f = fopen(argv[2], "rt");
			if (!f) {
				perror(argv[2]);
				SDL_Quit();
				return 1;
			}

			if (argc == 4) {
				fo = fopen(argv[3], "a");
				if (!fo) {
					perror(argv[3]);
					SDL_Quit();
					return 1;
				}
			} else {
				fo = stdout;
			}

			char line[1024];
			unsigned n_mapping = 0;
			while (fgets(line, sizeof(line), f)) {
				// Ignore comments or blank lines
				if (line[0] == '#' || line[0] == '\n')
					continue;

				for (int i = 0; i < numJoysticks; i++) {
					if (strncmp(line, guid[i], 32) == 0) {
						fprintf(fo, "%s\n", line);
						n_mapping++;
					}
				}
			}
			fflush(fo);
			if (argc == 4) {
				fclose(fo);
				if (n_mapping) 
					printf("[sdlGamepadMapper.c] Copy entry %s from \"%s\" to \"%s\"\n", guid[0], argv[2], argv[3]);
				else
					printf("[sdlGamepadMapper.c] No entry for %s copied from \"%s\" to \"%s\"\n", guid[0], argv[2], argv[3]);
			}
			fclose(f);
		}

		if (strcmp(argv[1], "--guid") == 0) {
			SDL_Quit();
			return 0;
		}
	}

	SDL_Window* win = SDL_CreateWindow("GamepadMapper", SDL_WINDOWPOS_CENTERED, SDL_WINDOWPOS_CENTERED, WINDOW_WIDTH, WINDOW_HEIGHT, SDL_WINDOW_SHOWN | SDL_WINDOW_FULLSCREEN);
	if (win == NULL) {
		printf("SDL_CreateWindow Error: %s\n", SDL_GetError());
		SDL_Quit();
		return 1;
	}

	SDL_Renderer* ren = SDL_CreateRenderer(win, -1, SDL_RENDERER_ACCELERATED);
	if (ren == NULL) {
		SDL_DestroyWindow(win);
		printf("SDL_CreateRenderer Error: %s\n", SDL_GetError());
		SDL_Quit();
		return 1;
	}

	// Initialize font cache
	init_font_cache(ren);

	SDL_version compiled;
	SDL_version linked;

	SDL_VERSION(&compiled);
	SDL_GetVersion(&linked);

	char str_buffer[3000];  // Add buffer to store version string
	char* str_ptr = str_buffer;
	char* str_end = str_buffer + sizeof(str_buffer);
	append(&str_ptr, str_end,
		"SDL Version (compiled): %d.%d.%d\nSDL Version (linked):   %d.%d.%d",
		compiled.major, compiled.minor, compiled.patch,
		linked.major, linked.minor, linked.patch);

	// Add SDL platform to string buffer
	append(&str_ptr, str_end,
		"\nSDL Platform: %s", SDL_GetPlatform());

	// Add number of video drivers and their names then append to string buffer
	int num_drivers = SDL_GetNumVideoDrivers();
	append(&str_ptr, str_end,
		"\n\nNumber of video drivers: %d (using %s)", num_drivers, SDL_GetCurrentVideoDriver());
	for (int i = 0; i < num_drivers; ++i) {
		const char* driver_name = SDL_GetVideoDriver(i);
		append(&str_ptr, str_end,
			"\n %d. %s", i + 1, driver_name);
	}

	// Add number of audio drivers and their names then append to string buffer
	int num_audio_drivers = SDL_GetNumAudioDrivers();
	append(&str_ptr, str_end,
		"\n\nNumber of audio drivers: %d (using %s)", num_audio_drivers, SDL_GetCurrentAudioDriver());
	for (int i = 0; i < num_audio_drivers; ++i) {
		const char* audio_driver_name = SDL_GetAudioDriver(i);
		append(&str_ptr, str_end,
			"\n %d. %s", i + 1, audio_driver_name);
	}

	// Add number of render drivers and their names	to string buffer
	int num_render_drivers = SDL_GetNumRenderDrivers();
	append(&str_ptr, str_end,
		"\n\nNumber of render drivers: %d", num_render_drivers);
	for (int i = 0; i < num_render_drivers; ++i) {
		SDL_RendererInfo info;
		if (SDL_GetRenderDriverInfo(i, &info) == 0) {
			append(&str_ptr, str_end,
				"\n %d. %s", i + 1, info.name);
		} else {
			append(&str_ptr, str_end,
				"\n %d: Error retrieving info", i);
		}
	}

	// Add number of displays and their names to string buffer
	int num_displays = SDL_GetNumVideoDisplays();
	append(&str_ptr, str_end,
		"\n\nNumber of displays: %d", num_displays);
	for (int i = 0; i < num_displays; ++i) {
		const char* displayName = SDL_GetDisplayName(i);
		SDL_Rect bounds;
		char bounds_info[100];
		if (SDL_GetDisplayBounds(i, &bounds) == 0) {
			snprintf(bounds_info, sizeof(bounds_info), "%d x %d", bounds.w, bounds.h);
		} else {
			snprintf(bounds_info, sizeof(bounds_info), "unknown bounds");
		}
		append(&str_ptr, str_end,
			"\n %d. %s (%s)", i + 1, displayName ? displayName : "Unknown", bounds_info);

	}

	// Print joystick information
	int numJoysticks = SDL_NumJoysticks();
	append(&str_ptr, str_end,
		"\n\nNumber of joysticks: %d\n", numJoysticks);

	for (int i = 0; i < numJoysticks; i++) {
		SDL_Joystick* joy = SDL_JoystickOpen(i);
		if (joy) {
			char guid[64];
			SDL_JoystickGetGUIDString(SDL_JoystickGetGUID(joy), guid, sizeof(guid));
			append(&str_ptr, str_end, "  %i. %s, GUID %s (%s)\n", i + 1, SDL_JoystickName(joy), guid, SDL_IsGameController(i) ? "GameController" : "Not mapped");
			append(&str_ptr, str_end, "  Axes %02d / Buttons %02d / Hats %02d / Balls %02d\n",
				SDL_JoystickNumAxes(joy), SDL_JoystickNumButtons(joy),
				SDL_JoystickNumHats(joy), SDL_JoystickNumBalls(joy));
			SDL_JoystickClose(joy);
		} else {
			printf("Sys_InitInput: SDL_JoystickOpen() failed: %s\n", SDL_GetError());
		}
	}

	SDL_Joystick* joy = SDL_JoystickOpen(0); // Open the first joystick for event handling
	// Storage for results
	const size_t N = sizeof(kOrder) / sizeof(kOrder[0]);
	MappingEntry* results = (MappingEntry*) calloc(N, sizeof(MappingEntry));
	if (!results) {
		fprintf(stderr, "Out of memory\n");
		SDL_JoystickClose(joy);
		SDL_DestroyWindow(win);
		SDL_Quit();
		return 1;
	}

	Uint32 startTime = SDL_GetTicks();
	Uint32 startTime2 = SDL_GetTicks();
	Uint32 currentTime = startTime;
	Uint32 currentTime2 = startTime2;

	int frameCount = 0;
	char fps_info[50];
	char wait_input_info[100];

	SDL_Event e;
	int quit = 0;
	size_t i = 0;
	MappingEntry* out = &results[i];
	const LogicalInput* li = &kOrder[i];
	int wait_release = 0;

	while (!quit) {
		while (!quit && SDL_PollEvent(&e)) {
			// printf("Event type: %d\n", e.type);
			if (e.type == SDL_QUIT) {
				quit = 1;
			} else if (e.type == SDL_KEYDOWN) {
				if (e.key.keysym.sym == SDLK_ESCAPE) {
					quit = 1;
				}
			}

			if (e.type == SDL_JOYHATMOTION) {
				if (e.jhat.value != SDL_HAT_CENTERED) {
					// Handle joystick hat motion
					out->assigned = 1;
					out->type = MAP_HAT;
					out->index = e.jhat.hat;
					out->direction = e.jhat.value; // bitmask
					printf("Captured hat h%d.%d for %s\n", out->index, out->direction, li->logical);
					wait_release = 1;
				} else {
					if (out->type == MAP_HAT && wait_release) {
						i++;
						wait_release = 0;
						if (i >= N) {
							quit = 1;
							printf("[hat] All done!\n");
						} else {
							startTime2 = SDL_GetTicks(); // reset timeout
							li = &kOrder[i];
							out = &results[i];
						}
					}
				}
				// printf("Joystick %d Hat %d Motion: %d\n", e.jhat.which, e.jhat.hat, e.jhat.value);
			} else if (e.type == SDL_JOYBUTTONDOWN) {
				// Handle joystick button press
				out->assigned = 1;
				out->type = MAP_BUTTON;
				out->index = e.jbutton.button;
				out->direction = 0;
				wait_release = 1;
				printf("Captured button b%d for %s\n", out->index, li->logical);
				// printf("Joystick %d Button %d Down\n", e.jbutton.which, e.jbutton.button);
			} else if (e.type == SDL_JOYBUTTONUP) {
				// Handle joystick button release
				if (out->type == MAP_BUTTON && wait_release) {
					i++;
					wait_release = 0;
					if (i >= N) {
						quit = 1;
						printf("[button] All done!\n");
					} else {
						startTime2 = SDL_GetTicks(); // reset timeout
						li = &kOrder[i];
						out = &results[i];
					}
				}
				// printf("Joystick %d Button %d Up\n", e.jbutton.which, e.jbutton.button);
			} else if (e.type == SDL_JOYAXISMOTION) {
				// Handle joystick axis motion
				if (SDL_abs(e.jaxis.value) > AXIS_DEADZONE) {
					out->assigned = 1;
					out->type = MAP_AXIS;
					out->index = e.jaxis.axis;
					out->direction = (e.jaxis.value < 0) ? -1 : +1;
					printf("Captured axis a%d for %s\n", out->index, li->logical);
					i++;
					if (i >= N) {
						quit = 1;
						printf("[axis] All done!\n");
					} else {
						startTime2 = SDL_GetTicks(); // reset timeout
						li = &kOrder[i];
						out = &results[i];
					}
					wait_for_release();
				}
				// printf("Joystick %d Axis %d Motion: %d\n", e.jaxis.which, e.jaxis.axis, e.jaxis.value);
			} else if (e.type == SDL_JOYDEVICEADDED) {
				// Handle joystick device added
				printf("Joystick device added: %d\n", e.jdevice.which);
			}
		}

		// FPS calculation
		frameCount++;
		currentTime = SDL_GetTicks();
		if (currentTime - startTime >= 1000) { // 1 second elapsed
			snprintf(fps_info, sizeof(fps_info), "FPS: %d\n", frameCount);
			frameCount = 0;
			startTime = currentTime;
			snprintf(wait_input_info, sizeof(wait_input_info), "Skip for %d seconds", (INPUT_TIMEOUT - currentTime2 + startTime2) / 1000);
		}

		// Input timeout handling
		currentTime2 = SDL_GetTicks();
		if (currentTime2 - startTime2 >= INPUT_TIMEOUT) { // 5 seconds elapsed
			i++;
			if (i >= N) {
				quit = 1;
				printf("All done!\n");
			} else {
				li = &kOrder[i];
				out = &results[i];
			}
			startTime2 = currentTime2;
		}

		if (!quit) {
			// Clear the renderer and draw the strings
			SDL_SetRenderDrawColor(ren, 0, 0, 0, 255);
			SDL_RenderClear(ren);
			SDL_Color color = { 0, 0, 255, 255 };
			draw_string_scaled(ren, "Gamepad Mapper", 10, 10, 2, color);
			color = (SDL_Color){ 255, 255, 255, 255 };
			draw_string(ren, str_buffer, 10, 30, color);
			color = (SDL_Color){ 0, 255, 0, 255 };
			draw_string(ren, fps_info, WINDOW_WIDTH - 70, 10, color);
			color = (SDL_Color){ 255, 255, 0, 255 };
			draw_string(ren, wait_input_info, 10, WINDOW_HEIGHT - 50, color);
			draw_string_scaled(ren, li->prompt, 10, WINDOW_HEIGHT - 40, 2, color);
			SDL_RenderPresent(ren);
			SDL_Delay(16);
		}
	}

	// Build mapping line
	char guidstr[64] = { 0 };
	SDL_JoystickGUID guid = SDL_JoystickGetGUID(joy);
	SDL_JoystickGetGUIDString(guid, guidstr, sizeof(guidstr));
	const char* joyname = SDL_JoystickName(joy);
	if (!joyname) joyname = "Unknown Controller";

	// check arguments for output file
	const char* out_filename = NULL;
	if (argc > 1) {
		out_filename = argv[1];
	} else {
		out_filename = "gamecontrollerdb.txt"; // default
	}
	FILE* f = fopen(out_filename, "a");
	if (f) {
		fprintf(f, "%s,%s,platform:%s", guidstr, joyname, SDL_GetPlatform());
		for (size_t i = 0; i < N; ++i) {
			if (!results[i].assigned) continue;
			char token[32];
			token_from_entry(&results[i], token, sizeof(token));
			fprintf(f, ",%s:%s", kOrder[i].logical, token);
		}
		fprintf(f, "\n");
		fclose(f);
		printf("[sdlGamepadMapper.c] Mapping %s appended to file: %s\n", guidstr, out_filename);
	}

	free(results);
	if (joy) SDL_JoystickClose(joy);
	free_font_cache();
	SDL_DestroyRenderer(ren);
	SDL_DestroyWindow(win);
	SDL_Quit();

	return 0;
}
