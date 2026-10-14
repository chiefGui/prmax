#include "ui.h"

#include <math.h>
#include <string.h>

#include "host.h"
#include "views.h"

#define DRAG_THRESHOLD 4.0f
#define PANEL_SECONDS 0.18f
#define WHEEL_SCALE 0.5f
#define OFFSCREEN -1000.0f

static float view_w, view_h;
static float pointer_x = OFFSCREEN, pointer_y = OFFSCREEN;
static int pointer_down;
static float wheel;
static double last_frame;
static float panel_open;
static uint32_t hovered;

static struct {
    int pending;
    int active;
    PrRef pr;
    float start_x, start_y, offset_x, offset_y, width;
} drag;

void ui_init(void)
{
    kit_init();
    app_init();
    last_frame = host_now();
}

void ui_theme(int light)
{
    kit_theme(light);
    host_redraw();
}

void ui_resize(float width, float height)
{
    view_w = width;
    view_h = height;
    kit_resize(width, height);
}

static void build(void)
{
    const Pr *lifted = drag.active ? app_pr(drag.pr) : 0;
    Lift lift = {lifted, pointer_x - drag.offset_x, pointer_y - drag.offset_y, drag.width};
    CLAY({.id = CLAY_ID("root"), .layout = {.sizing = {CLAY_SIZING_GROW(0), CLAY_SIZING_GROW(0)}, .layoutDirection = CLAY_TOP_TO_BOTTOM},
        .backgroundColor = th.bg}) {
        topbar_view();
        CLAY({.id = CLAY_ID("body"), .layout = {.sizing = {CLAY_SIZING_GROW(0), CLAY_SIZING_GROW(0)}}}) {
            board_view(lifted ? &lift : 0);
            if (panel_open > 0.0f) {
                panel_view(panel_open);
            }
        }
        if (app.dialog.open) {
            dialog_view(view_w, view_h);
        }
        if (lifted) {
            lift_view(&lift);
        }
        toast_view();
    }
}

void ui_paint(void)
{
    double now = host_now();
    float dt = fminf((float)(now - last_frame), 0.1f);
    float target = app.selection.open ? 1.0f : 0.0f;
    last_frame = now;
    panel_open = target > panel_open ? fminf(target, panel_open + dt / PANEL_SECONDS) : fmaxf(target, panel_open - dt / PANEL_SECONDS);
    Clay_SetPointerState((Clay_Vector2){pointer_x, pointer_y}, pointer_down);
    Clay_UpdateScrollContainers(false, (Clay_Vector2){0, wheel}, dt);
    wheel = 0;
    kit_render(kit_layout(build), now);
}

int ui_animating(void)
{
    if (panel_open != (app.selection.open ? 1.0f : 0.0f)) {
        return ANIMATE_SMOOTH;
    }
    for (int i = 0; i < app.state.count; i++) {
        if (app_visible(&app.state.prs[i]) && !strcmp(app.state.prs[i].status, "reviewing")) {
            return ANIMATE_AMBIENT;
        }
    }
    return ANIMATE_NONE;
}

static void activate(const Hit *h)
{
    Pr *p = app_pr(h->pr);
    switch (h->kind) {
    case HIT_REPO: app_toggle_repo(h->index); return;
    case HIT_ROUND: app_pick_round(h->index); return;
    case HIT_CLOSE: app_close(); return;
    case HIT_DISMISS: dialog_close(); return;
    case HIT_SUBMIT: dialog_submit(); return;
    case HIT_OPTION: dialog_choose(h->index / OPTION_MAX, h->index % OPTION_MAX); return;
    case HIT_PREVIOUS: dialog_step(h->index, -1); return;
    case HIT_NEXT: dialog_step(h->index, 1); return;
    }
    if (!p) {
        return;
    }
    switch (h->kind) {
    case HIT_CARD: app_select(p); break;
    case HIT_FINDING: app_open_finding(p, h->index); break;
    case HIT_OPEN: host_open(p->url); break;
    case HIT_REVIEW: dialog_open(p); break;
    case HIT_CANCEL: app_action(p, "cancel"); break;
    case HIT_NUDGE: app_action(p, "nudge"); app_toast("Nudging the PR's session…"); break;
    case HIT_POST: app_action(p, "post"); app_toast("Posting the review…"); break;
    }
}

static void route(uint32_t id, uint32_t type, uint32_t flags)
{
    if (!id) {
        return;
    }
    VogueEffects e = kit_route(id, type, flags);
    if ((e.flags & VOGUE_EFFECT_RELEASE_POINTER) && !drag.active) {
        host_capture(0);
    }
    if (e.flags & VOGUE_EFFECT_CAPTURE_POINTER) {
        host_capture(1);
    }
    if (e.flags & VOGUE_EFFECT_ACTIVATE) {
        const Hit *h = kit_hit(id);
        if (h) {
            activate(h);
        }
    }
}

static uint32_t inside(uint32_t id) { return kit_top_hit() == id ? VOGUE_EVENT_INSIDE : 0; }

static void refresh_hover(void)
{
    uint32_t over = kit_top_hit();
    if (over != hovered) {
        route(hovered, VOGUE_EVENT_POINTER_MOVE, 0);
        route(over, VOGUE_EVENT_POINTER_MOVE, VOGUE_EVENT_INSIDE);
        hovered = over;
    }
}

static void set_pointer(float x, float y)
{
    pointer_x = x;
    pointer_y = y;
    Clay_SetPointerState((Clay_Vector2){x, y}, pointer_down);
}

void ui_pointer_move(float x, float y)
{
    set_pointer(x, y);
    if (drag.pending && !drag.active && hypotf(x - drag.start_x, y - drag.start_y) > DRAG_THRESHOLD) {
        uint32_t captured = kit_captured();
        drag.active = 1;
        route(captured, VOGUE_EVENT_POINTER_CANCEL, 0);
        host_capture(1);
    }
    if (!drag.active) {
        uint32_t captured = kit_captured();
        if (captured) {
            route(captured, VOGUE_EVENT_POINTER_MOVE, inside(captured));
        } else {
            refresh_hover();
        }
    }
    host_redraw();
}

void ui_pointer_leave(void)
{
    if (!pointer_down) {
        ui_pointer_move(OFFSCREEN, OFFSCREEN);
    }
}

void ui_pointer_down(float x, float y, int shift)
{
    pointer_down = 1;
    set_pointer(x, y);
    uint32_t over = kit_top_hit();
    const Hit *h = kit_hit(over);
    if (h && h->kind == HIT_CARD) {
        Clay_BoundingBox box = Clay_GetElementData((Clay_ElementId){.id = over}).boundingBox;
        drag.pending = 1;
        drag.pr = h->pr;
        drag.start_x = x;
        drag.start_y = y;
        drag.offset_x = x - box.x;
        drag.offset_y = y - box.y;
        drag.width = box.width;
    }
    route(over, VOGUE_EVENT_POINTER_DOWN, VOGUE_EVENT_INSIDE | (shift ? VOGUE_EVENT_SHIFT : 0));
    host_redraw();
}

void ui_pointer_up(float x, float y, int shift)
{
    pointer_down = 0;
    set_pointer(x, y);
    if (drag.active) {
        Pr *p = app_pr(drag.pr);
        int column = board_column_at();
        memset(&drag, 0, sizeof drag);
        host_capture(0);
        if (p && column >= 0) {
            app_drop(p, column, shift);
        }
    } else {
        uint32_t captured = kit_captured();
        drag.pending = 0;
        route(captured, VOGUE_EVENT_POINTER_UP, inside(captured));
    }
    refresh_hover();
    host_redraw();
}

void ui_capture_lost(void)
{
    route(kit_captured(), VOGUE_EVENT_POINTER_CANCEL, 0);
    memset(&drag, 0, sizeof drag);
    host_redraw();
}

void ui_wheel(float delta)
{
    wheel += delta * WHEEL_SCALE;
    host_redraw();
}

void ui_key(int key)
{
    if (key == KEY_ESCAPE) {
        if (app.dialog.open) {
            dialog_close();
        } else {
            app_close();
        }
    } else if (key == KEY_ENTER) {
        dialog_submit();
    }
    host_redraw();
}
