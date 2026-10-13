#ifndef PRMAX_GFX_H
#define PRMAX_GFX_H

#include <windows.h>

#ifdef __cplusplus
extern "C" {
#endif

enum {
    FONT_TEXT = 0,
    FONT_MEDIUM,
    FONT_MONO,
    FONT_ICON
};

typedef struct Rgba {
    float r, g, b, a;
} Rgba;

int gfx_init(HWND hwnd);
void gfx_shutdown(void);
void gfx_resize(UINT width, UINT height, float dpi);
void gfx_begin(Rgba clear);
void gfx_end(void);
void gfx_fill(float x, float y, float w, float h, float radius, Rgba color);
void gfx_stroke(float x, float y, float w, float h, float radius, float width, Rgba color);
void gfx_text(const char *utf8, int length, float x, float y, float w, float h, int font, float size, Rgba color);
float gfx_measure(const char *utf8, int length, int font, float size);
void gfx_clip(float x, float y, float w, float h);
void gfx_unclip(void);

#ifdef __cplusplus
}
#endif

#endif
