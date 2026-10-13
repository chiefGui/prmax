#include "gfx.h"

#include <d2d1.h>
#include <dwrite.h>
#include <stdlib.h>

static ID2D1Factory *factory;
static IDWriteFactory *dwrite;
static ID2D1HwndRenderTarget *target;
static ID2D1SolidColorBrush *brush;
static HWND window;
static float scale = 1.0f;

struct Format {
    int font;
    int size;
    IDWriteTextFormat *format;
};

static Format formats[32];
static int formatCount;

static const wchar_t *family(int font)
{
    switch (font) {
    case FONT_MONO: return L"Cascadia Mono";
    case FONT_ICON: return L"Segoe Fluent Icons";
    default: return L"Segoe UI Variable Text";
    }
}

static IDWriteTextFormat *format(int font, float size)
{
    int key = (int)(size * 4.0f + 0.5f);
    for (int i = 0; i < formatCount; i++) {
        if (formats[i].font == font && formats[i].size == key) {
            return formats[i].format;
        }
    }
    if (formatCount == (int)(sizeof formats / sizeof formats[0])) {
        return 0;
    }
    IDWriteTextFormat *f = 0;
    DWRITE_FONT_WEIGHT weight = font == FONT_MEDIUM ? DWRITE_FONT_WEIGHT_SEMI_BOLD : DWRITE_FONT_WEIGHT_NORMAL;
    if (FAILED(dwrite->CreateTextFormat(family(font), 0, weight, DWRITE_FONT_STYLE_NORMAL,
            DWRITE_FONT_STRETCH_NORMAL, size, L"en-us", &f))) {
        return 0;
    }
    f->SetWordWrapping(DWRITE_WORD_WRAPPING_NO_WRAP);
    f->SetParagraphAlignment(DWRITE_PARAGRAPH_ALIGNMENT_CENTER);
    formats[formatCount++] = {font, key, f};
    return f;
}

struct Wide {
    wchar_t stack[512];
    wchar_t *text;
    int length;

    Wide(const char *utf8, int n) : text(stack), length(0)
    {
        if (n <= 0) {
            return;
        }
        int need = MultiByteToWideChar(CP_UTF8, 0, utf8, n, 0, 0);
        if (need > (int)(sizeof stack / sizeof stack[0])) {
            text = (wchar_t *)malloc(sizeof(wchar_t) * need);
        }
        length = MultiByteToWideChar(CP_UTF8, 0, utf8, n, text, need);
    }

    ~Wide()
    {
        if (text != stack) {
            free(text);
        }
    }
};

static D2D1_COLOR_F color(Rgba c) { return D2D1::ColorF(c.r, c.g, c.b, c.a); }

static void release()
{
    if (brush) { brush->Release(); brush = 0; }
    if (target) { target->Release(); target = 0; }
}

static bool create()
{
    if (target) {
        return true;
    }
    RECT rc;
    GetClientRect(window, &rc);
    D2D1_RENDER_TARGET_PROPERTIES props = D2D1::RenderTargetProperties();
    props.dpiX = props.dpiY = 96.0f * scale;
    D2D1_HWND_RENDER_TARGET_PROPERTIES hp = D2D1::HwndRenderTargetProperties(window,
        D2D1::SizeU(rc.right - rc.left, rc.bottom - rc.top));
    if (FAILED(factory->CreateHwndRenderTarget(props, hp, &target))) {
        return false;
    }
    target->SetTextAntialiasMode(D2D1_TEXT_ANTIALIAS_MODE_CLEARTYPE);
    target->CreateSolidColorBrush(D2D1::ColorF(0, 0, 0), &brush);
    return true;
}

extern "C" int gfx_init(HWND hwnd)
{
    window = hwnd;
    if (FAILED(D2D1CreateFactory(D2D1_FACTORY_TYPE_SINGLE_THREADED, &factory))) {
        return 0;
    }
    if (FAILED(DWriteCreateFactory(DWRITE_FACTORY_TYPE_SHARED, __uuidof(IDWriteFactory), (IUnknown **)&dwrite))) {
        return 0;
    }
    return 1;
}

extern "C" void gfx_shutdown(void)
{
    release();
    for (int i = 0; i < formatCount; i++) {
        formats[i].format->Release();
    }
    formatCount = 0;
    if (dwrite) { dwrite->Release(); dwrite = 0; }
    if (factory) { factory->Release(); factory = 0; }
}

extern "C" void gfx_resize(UINT width, UINT height, float dpi)
{
    float s = dpi / 96.0f;
    if (s != scale) {
        scale = s;
        release();
    }
    if (target) {
        target->Resize(D2D1::SizeU(width, height));
    }
}

extern "C" void gfx_begin(Rgba clear)
{
    if (!create()) {
        return;
    }
    target->BeginDraw();
    target->SetTransform(D2D1::Matrix3x2F::Identity());
    target->Clear(color(clear));
}

extern "C" void gfx_end(void)
{
    if (target && target->EndDraw() == D2DERR_RECREATE_TARGET) {
        release();
        InvalidateRect(window, 0, FALSE);
    }
}

extern "C" void gfx_fill(float x, float y, float w, float h, float radius, Rgba c)
{
    if (!target || c.a <= 0.0f) {
        return;
    }
    brush->SetColor(color(c));
    D2D1_RECT_F r = D2D1::RectF(x, y, x + w, y + h);
    if (radius > 0.0f) {
        target->FillRoundedRectangle(D2D1::RoundedRect(r, radius, radius), brush);
    } else {
        target->FillRectangle(r, brush);
    }
}

extern "C" void gfx_stroke(float x, float y, float w, float h, float radius, float width, Rgba c)
{
    if (!target || c.a <= 0.0f) {
        return;
    }
    brush->SetColor(color(c));
    float i = width * 0.5f;
    D2D1_RECT_F r = D2D1::RectF(x + i, y + i, x + w - i, y + h - i);
    if (radius > 0.0f) {
        target->DrawRoundedRectangle(D2D1::RoundedRect(r, radius - i, radius - i), brush, width);
    } else {
        target->DrawRectangle(r, brush, width);
    }
}

extern "C" void gfx_text(const char *utf8, int length, float x, float y, float w, float h, int font, float size, Rgba c)
{
    if (!target || length <= 0) {
        return;
    }
    IDWriteTextFormat *f = format(font, size);
    if (!f) {
        return;
    }
    Wide wide(utf8, length);
    brush->SetColor(color(c));
    target->DrawText(wide.text, wide.length, f, D2D1::RectF(x, y, x + w, y + h), brush,
        D2D1_DRAW_TEXT_OPTIONS_NONE);
}

extern "C" float gfx_measure(const char *utf8, int length, int font, float size)
{
    IDWriteTextFormat *f = format(font, size);
    IDWriteTextLayout *layout = 0;
    DWRITE_TEXT_METRICS m;
    float width = 0.0f;
    if (!f || length <= 0) {
        return width;
    }
    Wide wide(utf8, length);
    if (SUCCEEDED(dwrite->CreateTextLayout(wide.text, wide.length, f, 1e6f, 1e6f, &layout))) {
        if (SUCCEEDED(layout->GetMetrics(&m))) {
            width = m.widthIncludingTrailingWhitespace;
        }
        layout->Release();
    }
    return width;
}

extern "C" void gfx_clip(float x, float y, float w, float h)
{
    if (target) {
        target->PushAxisAlignedClip(D2D1::RectF(x, y, x + w, y + h), D2D1_ANTIALIAS_MODE_ALIASED);
    }
}

extern "C" void gfx_unclip(void)
{
    if (target) {
        target->PopAxisAlignedClip();
    }
}
