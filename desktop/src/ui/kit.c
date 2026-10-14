#include "kit.h"

#include <math.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define HIT_MAX 1024

Theme th;
float kit_fade = 1.0f;
const Clay_Color CLEAR = {0, 0, 0, 0};
int kit_bar, kit_shadow;

static char arena[1 << 18];
static size_t arena_used;
static Clay_Dimensions viewport;
static Hit hits[2][HIT_MAX];
static int hit_count[2];
static int building, ready = 1;
static VogueContext vogue;

static Clay_Color hex(uint32_t rgb, float a)
{
    return (Clay_Color){(float)((rgb >> 16) & 255), (float)((rgb >> 8) & 255), (float)(rgb & 255), 255.0f * a};
}

void kit_theme(int light)
{
    if (light) {
        th = (Theme){
            .bg = hex(0xF4F4F5, 1), .panel = hex(0xFFFFFF, 1), .card = hex(0xFFFFFF, 1), .column = hex(0xE9E9EC, 1),
            .hover = hex(0x000000, 0.05f), .drop = hex(0x2F6FE4, 0.08f), .border = hex(0x000000, 0.09f), .border_hover = hex(0x000000, 0.18f),
            .text = hex(0x1C1C1F, 1), .secondary = hex(0x5C5C63, 1), .muted = hex(0x96969D, 1), .accent = hex(0x2F6FE4, 1), .on_accent = hex(0xFFFFFF, 1),
            .warning = hex(0xB26A00, 1), .success = hex(0x23854D, 1), .pro = hex(0x6B4FD6, 1), .danger = hex(0xC93B33, 1),
            .overlay = hex(0x000000, 0.25f), .toast = hex(0x1C1C1F, 1), .toast_text = hex(0xF4F4F5, 1), .shadow = hex(0x000000, 0.06f)
        };
    } else {
        th = (Theme){
            .bg = hex(0x18181A, 1), .panel = hex(0x1E1E21, 1), .card = hex(0x242427, 1), .column = hex(0x2C2C30, 1),
            .hover = hex(0xFFFFFF, 0.06f), .drop = hex(0x4C8DFF, 0.10f), .border = hex(0xFFFFFF, 0.07f), .border_hover = hex(0xFFFFFF, 0.16f),
            .text = hex(0xE8E8EA, 1), .secondary = hex(0xA3A3AA, 1), .muted = hex(0x6F6F77, 1), .accent = hex(0x5B95FF, 1), .on_accent = hex(0xFFFFFF, 1),
            .warning = hex(0xE3A33B, 1), .success = hex(0x53B87C, 1), .pro = hex(0x9C82F2, 1), .danger = hex(0xEA6158, 1),
            .overlay = hex(0x000000, 0.45f), .toast = hex(0xE8E8EA, 1), .toast_text = hex(0x18181A, 1), .shadow = hex(0x000000, 0.18f)
        };
    }
}

static Clay_Dimensions measure(Clay_StringSlice s, Clay_TextElementConfig *config, void *user)
{
    (void)user;
    return (Clay_Dimensions){gfx_measure(s.chars, s.length, config->fontId, (float)config->fontSize), (float)config->lineHeight};
}

static void report(Clay_ErrorData e)
{
    char message[256];
    snprintf(message, sizeof message, "clay: %.*s\n", e.errorText.length, e.errorText.chars);
    OutputDebugStringA(message);
}

void kit_init(void)
{
    uint32_t size = Clay_MinMemorySize();
    vogue_context_init(&vogue);
    Clay_Initialize(Clay_CreateArenaWithCapacityAndMemory(size, malloc(size)), (Clay_Dimensions){0, 0}, (Clay_ErrorHandler){report, 0});
    Clay_SetMeasureTextFunction(measure, 0);
    Clay_SetCullingEnabled(false);
}

void kit_resize(float width, float height)
{
    viewport = (Clay_Dimensions){width, height};
    Clay_SetLayoutDimensions(viewport);
}

static int visible(Clay_BoundingBox b)
{
    return b.x < viewport.width && b.y < viewport.height && b.x + b.width > 0 && b.y + b.height > 0;
}

Clay_RenderCommandArray kit_layout(void (*build)(void))
{
    arena_used = 0;
    hit_count[building] = 0;
    Clay_BeginLayout();
    build();
    Clay_RenderCommandArray commands = Clay_EndLayout();
    ready = building;
    building ^= 1;
    return commands;
}

Clay_Color fade(Clay_Color c)
{
    c.a *= kit_fade;
    return c;
}

Clay_String S(const char *s)
{
    return (Clay_String){.length = (int32_t)strlen(s), .chars = s};
}

Clay_String F(const char *fmt, ...)
{
    char *out = arena + arena_used;
    size_t room = sizeof arena - arena_used;
    va_list ap;
    va_start(ap, fmt);
    int n = vsnprintf(out, room, fmt, ap);
    va_end(ap);
    n = n < 0 ? 0 : (size_t)n >= room ? (int)room - 1 : n;
    arena_used += (size_t)n + 1;
    return (Clay_String){.length = n, .chars = out};
}

static uint16_t line_height(int size) { return (uint16_t)(size + (size >= 13 ? 7 : 6)); }

static void label(Clay_String s, int font, int size, Clay_Color color, Clay_TextElementConfigWrapMode wrap)
{
    CLAY_TEXT(s, CLAY_TEXT_CONFIG({.textColor = fade(color), .fontId = (uint16_t)font, .fontSize = (uint16_t)size,
        .lineHeight = line_height(size), .wrapMode = wrap}));
}

void text(Clay_String s, int font, int size, Clay_Color color) { label(s, font, size, color, CLAY_TEXT_WRAP_WORDS); }

void line(Clay_String s, int font, int size, Clay_Color color) { label(s, font, size, color, CLAY_TEXT_WRAP_NONE); }

void gap(float height)
{
    CLAY({.layout = {.sizing = {.width = CLAY_SIZING_GROW(0), .height = CLAY_SIZING_FIXED(height)}}}) {}
}

void spacer(void)
{
    CLAY({.layout = {.sizing = {.width = CLAY_SIZING_GROW(0)}}}) {}
}

static VogueButton button(uint32_t id)
{
    VogueButton b;
    vogue_button_init(&b, id);
    return b;
}

uint32_t hit(Clay_ElementId id, int kind, int index, PrRef pr)
{
    if (hit_count[building] < HIT_MAX) {
        hits[building][hit_count[building]++] = (Hit){id.id, kind, index, pr};
    }
    VogueButton b = button(id.id);
    return vogue_button_state(&vogue, &b);
}

const Hit *kit_hit(uint32_t id)
{
    for (int i = 0; i < hit_count[ready]; i++) {
        if (hits[ready][i].id == id) {
            return &hits[ready][i];
        }
    }
    return 0;
}

uint32_t kit_top_hit(void)
{
    Clay_ElementIdArray over = Clay_GetPointerOverIds();
    uint32_t top = 0;
    for (int i = 0; i < over.length; i++) {
        if (kit_hit(over.internalArray[i].id)) {
            top = over.internalArray[i].id;
        }
    }
    return top;
}

VogueEffects kit_route(uint32_t id, uint32_t type, uint32_t flags)
{
    VogueButton b = button(id);
    VogueEvent ev = {.type = type, .button = VOGUE_POINTER_PRIMARY, .flags = flags};
    return vogue_button_handle(&vogue, &b, &ev);
}

uint32_t kit_captured(void) { return (uint32_t)vogue_context_captured(&vogue); }

static int hovered(uint32_t state) { return (state & VOGUE_STATE_HOVERED) != 0; }

void icon_button(Clay_ElementId id, int kind, int index, PrRef pr, const char *glyph)
{
    int over = hovered(hit(id, kind, index, pr));
    CLAY({.id = id, .layout = {.sizing = {CLAY_SIZING_FIXED(28), CLAY_SIZING_FIXED(28)},
        .childAlignment = {CLAY_ALIGN_X_CENTER, CLAY_ALIGN_Y_CENTER}}, .cornerRadius = CLAY_CORNER_RADIUS(6),
        .backgroundColor = over ? th.hover : CLEAR}) {
        line(S(glyph), FONT_ICON, 12, over ? th.text : th.secondary);
    }
}

void text_button(Clay_ElementId id, int kind, PrRef pr, const char *caption, int primary)
{
    int over = hovered(hit(id, kind, 0, pr));
    Clay_Color bg = primary ? th.accent : over ? th.hover : CLEAR;
    if (primary && over) {
        bg.a *= 0.88f;
    }
    CLAY({.id = id, .layout = {.padding = {11, 11, 5, 6}}, .cornerRadius = CLAY_CORNER_RADIUS(6), .backgroundColor = bg,
        .border = {.color = primary ? CLEAR : th.border_hover, .width = CLAY_BORDER_OUTSIDE(primary ? 0 : 1)}}) {
        line(S(caption), primary ? FONT_MEDIUM : FONT_TEXT, 12, primary ? th.on_accent : th.text);
    }
}

void pill(Clay_ElementId id, int kind, int index, PrRef pr, Clay_String caption, int on, Clay_Color color)
{
    int over = hovered(hit(id, kind, index, pr));
    if (color.a == 0) {
        color = on ? th.text : over ? th.secondary : th.muted;
    }
    CLAY({.id = id, .layout = {.padding = {9, 9, 4, 4}}, .cornerRadius = CLAY_CORNER_RADIUS(12),
        .backgroundColor = on ? th.column : over ? th.hover : CLEAR}) {
        line(caption, FONT_TEXT, 12, color);
    }
}

static Rgba rgba(Clay_Color c) { return (Rgba){c.r / 255.0f, c.g / 255.0f, c.b / 255.0f, c.a / 255.0f}; }

static float radius(Clay_CornerRadius r, Clay_BoundingBox b)
{
    float half = fminf(b.width, b.height) * 0.5f;
    return fminf(r.topLeft, half);
}

static void shadow(Clay_BoundingBox b, float r)
{
    Rgba c = rgba(th.shadow);
    for (int k = 3; k >= 1; k--) {
        float g = 3.0f * (float)k;
        gfx_fill(b.x - g * 0.5f, b.y + g * 0.6f, b.width + g, b.height + g * 0.6f, r + g * 0.5f, c);
    }
}

static void progress(Clay_BoundingBox b, Clay_Color color, double now)
{
    float phase = (float)fmod(now / 1.4, 1.0);
    float segment = b.width * 0.35f;
    float x = b.x - segment + (b.width + segment) * phase;
    float x0 = fmaxf(x, b.x), x1 = fminf(x + segment, b.x + b.width);
    if (x1 > x0) {
        gfx_fill(x0, b.y, x1 - x0, b.height, b.height * 0.5f, rgba(color));
    }
}

static void border(Clay_BoundingBox b, Clay_BorderRenderData *r)
{
    Rgba c = rgba(r->color);
    Clay_BorderWidth w = r->width;
    if (w.left && w.left == w.right && w.left == w.top && w.left == w.bottom) {
        gfx_stroke(b.x, b.y, b.width, b.height, radius(r->cornerRadius, b), w.left, c);
        return;
    }
    if (w.left) gfx_fill(b.x, b.y, w.left, b.height, 0, c);
    if (w.right) gfx_fill(b.x + b.width - w.right, b.y, w.right, b.height, 0, c);
    if (w.top) gfx_fill(b.x, b.y, b.width, w.top, 0, c);
    if (w.bottom) gfx_fill(b.x, b.y + b.height - w.bottom, b.width, w.bottom, 0, c);
}

void kit_render(Clay_RenderCommandArray commands, double now)
{
    gfx_begin(rgba(th.bg));
    for (int i = 0; i < commands.length; i++) {
        Clay_RenderCommand *cmd = Clay_RenderCommandArray_Get(&commands, i);
        Clay_BoundingBox b = cmd->boundingBox;
        int clip = cmd->commandType == CLAY_RENDER_COMMAND_TYPE_SCISSOR_START || cmd->commandType == CLAY_RENDER_COMMAND_TYPE_SCISSOR_END;
        if (!clip && !visible(b)) {
            continue;
        }
        switch (cmd->commandType) {
        case CLAY_RENDER_COMMAND_TYPE_RECTANGLE: {
            float r = radius(cmd->renderData.rectangle.cornerRadius, b);
            if (cmd->userData == &kit_shadow) {
                shadow(b, r);
            }
            gfx_fill(b.x, b.y, b.width, b.height, r, rgba(cmd->renderData.rectangle.backgroundColor));
            break;
        }
        case CLAY_RENDER_COMMAND_TYPE_BORDER:
            border(b, &cmd->renderData.border);
            break;
        case CLAY_RENDER_COMMAND_TYPE_TEXT: {
            Clay_TextRenderData *t = &cmd->renderData.text;
            gfx_text(t->stringContents.chars, t->stringContents.length, b.x, b.y, b.width, b.height, t->fontId,
                (float)t->fontSize, rgba(t->textColor));
            break;
        }
        case CLAY_RENDER_COMMAND_TYPE_SCISSOR_START:
            gfx_clip(b.x, b.y, b.width, b.height);
            break;
        case CLAY_RENDER_COMMAND_TYPE_SCISSOR_END:
            gfx_unclip();
            break;
        case CLAY_RENDER_COMMAND_TYPE_CUSTOM:
            if (cmd->renderData.custom.customData == &kit_bar) {
                progress(b, cmd->renderData.custom.backgroundColor, now);
            }
            break;
        default:
            break;
        }
    }
    gfx_end();
}
