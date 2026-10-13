#include "views.h"

#include <string.h>

#include "host.h"

static const char *COLUMN_NAMES[COLUMN_COUNT] = {"To review", "Reviewing", "Has issues", "Passed"};

void topbar_view(void)
{
    CLAY({.id = CLAY_ID("top"), .layout = {.sizing = {.width = CLAY_SIZING_GROW(0)}, .padding = {14, 18, 12, 6}, .childGap = 2,
        .childAlignment = {.y = CLAY_ALIGN_Y_CENTER}}}) {
        for (int i = 0; i < app.repos.count; i++) {
            const char *slash = strrchr(app.repos.name[i], '/');
            pill(CLAY_IDI("repo", i), HIT_REPO, i, (PrRef){0}, S(slash ? slash + 1 : app.repos.name[i]), !app.repos.hidden[i], CLEAR);
        }
        spacer();
        if (!app.connected) {
            line(S(app.ever_connected ? "Daemon offline" : "Connecting…"), FONT_TEXT, 12, app.ever_connected ? th.danger : th.muted);
        }
    }
}

static int mark(const Pr *p, Clay_String *out, Clay_Color *color)
{
    const Review *done = pr_last_done(p);
    *color = th.muted;
    switch (pr_column(p)) {
    case COLUMN_ISSUES:
        if (!done) {
            return 0;
        }
        *out = F("\xE2\x96\xB3 %d", done->finding_count);
        *color = th.warning;
        return 1;
    case COLUMN_REVIEWING:
        *out = S("queued");
        return !strcmp(p->status, "queued");
    case COLUMN_TODO:
        if (!strcmp(p->status, "failed")) {
            *out = S("failed");
            *color = th.danger;
            return 1;
        }
        *out = S(!strcmp(p->status, "outdated") ? "updated" : "draft");
        return !strcmp(p->status, "outdated") || p->draft;
    }
    return 0;
}

static void card(const Pr *p, int index, const Lift *lift, int floating)
{
    Clay_ElementId id = floating ? CLAY_ID("lifted") : CLAY_IDI("card", index);
    uint32_t state = floating ? 0 : hit(id, HIT_CARD, index, pr_ref(p));
    int selected = !floating && app.selection.open && pr_is(p, app.selection.pr);
    int lifted = !floating && lift && lift->pr == p;
    float saved = kit_fade;
    if (lifted) {
        kit_fade *= 0.25f;
    }
    Clay_Color edge = selected ? th.accent : floating || (state & VOGUE_STATE_HOVERED) ? th.border_hover : th.border;
    Clay_String note;
    Clay_Color note_color;
    int noted = mark(p, &note, &note_color);
    CLAY({.id = id, .layout = {.sizing = {.width = CLAY_SIZING_GROW(0)}, .layoutDirection = CLAY_TOP_TO_BOTTOM},
        .backgroundColor = fade(th.card), .cornerRadius = CLAY_CORNER_RADIUS(8),
        .border = {.color = fade(edge), .width = CLAY_BORDER_OUTSIDE(1)}, .userData = floating ? &kit_shadow : 0}) {
        CLAY({.layout = {.sizing = {.width = CLAY_SIZING_GROW(0)}, .layoutDirection = CLAY_TOP_TO_BOTTOM,
            .padding = {11, 11, 9, 10}, .childGap = 4}}) {
            CLAY({.layout = {.sizing = {.width = CLAY_SIZING_GROW(0)}, .childGap = 6, .childAlignment = {.y = CLAY_ALIGN_Y_CENTER}}}) {
                line(F("%s #%d", p->name, p->number), FONT_TEXT, 11, th.muted);
                spacer();
                if (noted) {
                    line(note, FONT_TEXT, 11, note_color);
                }
                if (p->unread) {
                    CLAY({.layout = {.sizing = {CLAY_SIZING_FIXED(6), CLAY_SIZING_FIXED(6)}}, .cornerRadius = CLAY_CORNER_RADIUS(3),
                        .backgroundColor = fade(th.accent)}) {}
                }
            }
            text(S(p->title), FONT_TEXT, 13, th.text);
        }
        if (!strcmp(p->status, "reviewing")) {
            CLAY({.layout = {.sizing = {.width = CLAY_SIZING_GROW(0), .height = CLAY_SIZING_FIXED(2)}, .padding = {8, 8, 0, 0}}}) {
                CLAY({.layout = {.sizing = {CLAY_SIZING_GROW(0), CLAY_SIZING_GROW(0)}}, .custom = {.customData = &kit_bar},
                    .backgroundColor = fade(th.pro)}) {}
            }
        }
    }
    kit_fade = saved;
}

static void column(int c, const Lift *lift)
{
    int from = lift ? pr_column(lift->pr) : -1;
    int target = lift && c != from && app_can_drop(lift->pr, c);
    int count = 0;
    for (int i = 0; i < app.state.count; i++) {
        count += app_visible(&app.state.prs[i]) && pr_column(&app.state.prs[i]) == c;
    }
    float saved = kit_fade;
    if (lift && c != from && !target) {
        kit_fade = 0.3f;
    }
    CLAY({.id = CLAY_IDI("column", c), .layout = {.sizing = {CLAY_SIZING_PERCENT(1.0f / COLUMN_COUNT), CLAY_SIZING_GROW(0)},
        .layoutDirection = CLAY_TOP_TO_BOTTOM, .padding = CLAY_PADDING_ALL(6)}, .cornerRadius = CLAY_CORNER_RADIUS(12),
        .backgroundColor = target ? th.drop : CLEAR}) {
        CLAY({.layout = {.padding = {5, 5, 2, 10}, .childGap = 7, .childAlignment = {.y = CLAY_ALIGN_Y_CENTER}}}) {
            line(S(COLUMN_NAMES[c]), FONT_MEDIUM, 12, th.secondary);
            if (count) {
                line(F("%d", count), FONT_TEXT, 12, th.muted);
            }
        }
        CLAY({.id = CLAY_IDI("list", c), .layout = {.sizing = {CLAY_SIZING_GROW(0), CLAY_SIZING_GROW(0)},
            .layoutDirection = CLAY_TOP_TO_BOTTOM, .childGap = 6}, .clip = {.vertical = true, .childOffset = Clay_GetScrollOffset()}}) {
            for (int i = 0; i < app.state.count; i++) {
                const Pr *p = &app.state.prs[i];
                if (app_visible(p) && pr_column(p) == c) {
                    card(p, i, lift, 0);
                }
            }
        }
    }
    kit_fade = saved;
}

void board_view(const Lift *lift)
{
    CLAY({.id = CLAY_ID("board"), .layout = {.sizing = {CLAY_SIZING_GROW(0), CLAY_SIZING_GROW(0)}, .padding = {10, 10, 2, 10}, .childGap = 6}}) {
        for (int c = 0; c < COLUMN_COUNT; c++) {
            column(c, lift);
        }
    }
}

int board_column_at(void)
{
    for (int c = 0; c < COLUMN_COUNT; c++) {
        if (Clay_PointerOver(CLAY_IDI("column", c))) {
            return c;
        }
    }
    return -1;
}

void lift_view(const Lift *lift)
{
    CLAY({.id = CLAY_ID("lift"), .layout = {.sizing = {.width = CLAY_SIZING_FIXED(lift->width)}},
        .floating = {.attachTo = CLAY_ATTACH_TO_ROOT, .offset = {lift->x, lift->y}, .zIndex = 200,
            .pointerCaptureMode = CLAY_POINTER_CAPTURE_MODE_PASSTHROUGH}}) {
        card(lift->pr, 0, 0, 1);
    }
}

void toast_view(void)
{
    if (!app.toast[0] || host_now() >= app.toast_until) {
        return;
    }
    CLAY({.id = CLAY_ID("toast"), .layout = {.padding = {14, 14, 8, 9}}, .backgroundColor = th.toast, .cornerRadius = CLAY_CORNER_RADIUS(8),
        .floating = {.attachTo = CLAY_ATTACH_TO_ROOT, .attachPoints = {.element = CLAY_ATTACH_POINT_CENTER_BOTTOM, .parent = CLAY_ATTACH_POINT_CENTER_BOTTOM},
            .offset = {0, -24}, .zIndex = 150, .pointerCaptureMode = CLAY_POINTER_CAPTURE_MODE_PASSTHROUGH}}) {
        line(S(app.toast), FONT_TEXT, 12, th.toast_text);
    }
}
