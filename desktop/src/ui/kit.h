#ifndef PRMAX_KIT_H
#define PRMAX_KIT_H

#include "clay.h"
#include "vogue.h"

#include "app/app.h"
#include "platform/gfx.h"

typedef struct Theme {
    Clay_Color bg, panel, card, column, hover, drop, border, border_hover;
    Clay_Color text, secondary, muted, accent, on_accent, warning, success, pro, danger;
    Clay_Color overlay, toast, toast_text, shadow;
} Theme;

typedef struct Hit {
    uint32_t id;
    int kind;
    int index;
    PrRef pr;
} Hit;

extern Theme th;
extern float kit_fade;
extern const Clay_Color CLEAR;
extern int kit_bar, kit_shadow;

void kit_init(void);
void kit_theme(int light);
Clay_RenderCommandArray kit_layout(void (*build)(void));
void kit_render(Clay_RenderCommandArray commands, double now);

Clay_Color fade(Clay_Color c);
Clay_String S(const char *s);
Clay_String F(const char *fmt, ...);
void text(Clay_String s, int font, int size, Clay_Color color);
void line(Clay_String s, int font, int size, Clay_Color color);
void gap(float height);
void spacer(void);

uint32_t hit(Clay_ElementId id, int kind, int index, PrRef pr);
const Hit *kit_hit(uint32_t id);
uint32_t kit_top_hit(void);
VogueEffects kit_route(uint32_t id, uint32_t type, uint32_t flags);
uint32_t kit_captured(void);

void icon_button(Clay_ElementId id, int kind, int index, PrRef pr, const char *glyph);
void text_button(Clay_ElementId id, int kind, PrRef pr, const char *label, int primary);
void pill(Clay_ElementId id, int kind, int index, PrRef pr, Clay_String label, int on, Clay_Color color);

#endif
