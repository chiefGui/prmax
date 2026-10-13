#include "views.h"

#include <string.h>

#define ICON_OPEN "\xEE\xA2\xA7"
#define ICON_CLOSE "\xEE\x9C\x91"

static const char *CATEGORIES[][2] = {
    {"bugs", "Bugs"}, {"cleanup", "Cleanup"}, {"performance", "Performance"},
    {"architecture", "Architecture"}, {"tests", "Tests"}, {"docs", "Docs"}
};

static void message(Clay_String s, Clay_Color color)
{
    CLAY({.layout = {.sizing = {.width = CLAY_SIZING_GROW(0)}, .padding = {8, 8, 8, 0}}}) {
        text(s, FONT_TEXT, 13, color);
    }
}

static void log_view(void)
{
    if (app.logs.count == 0) {
        message(S("Starting…"), th.muted);
        return;
    }
    CLAY({.layout = {.sizing = {.width = CLAY_SIZING_GROW(0)}, .layoutDirection = CLAY_TOP_TO_BOTTOM, .padding = {8, 8, 8, 0}, .childGap = 4}}) {
        for (int i = app.logs.count > 80 ? app.logs.count - 80 : 0; i < app.logs.count; i++) {
            const char *s = app.logs.lines[i];
            switch (app.logs.kinds[i]) {
            case LOG_TOOL: text(F("\xE2\x80\xBA %s", s), FONT_TEXT, 12, th.muted); break;
            case LOG_TEXT: text(S(s), FONT_TEXT, 12, th.secondary); break;
            case LOG_ERROR: text(S(s), FONT_TEXT, 12, th.danger); break;
            case LOG_DONE: text(S(s), FONT_TEXT, 12, th.success); break;
            default: text(S(s), FONT_TEXT, 12, th.muted); break;
            }
        }
    }
}

static void finding(const Pr *p, const Finding *f, int index)
{
    Clay_ElementId id = CLAY_IDI("finding", index);
    int over = (hit(id, HIT_FINDING, index, pr_ref(p)) & VOGUE_STATE_HOVERED) != 0;
    CLAY({.id = id, .layout = {.sizing = {.width = CLAY_SIZING_GROW(0)}, .layoutDirection = CLAY_TOP_TO_BOTTOM,
        .padding = {8, 8, 7, 8}, .childGap = 3}, .cornerRadius = CLAY_CORNER_RADIUS(6), .backgroundColor = over ? th.hover : CLEAR}) {
        text(S(f->title), FONT_MEDIUM, 13, th.text);
        text(S(f->text), FONT_TEXT, 12, th.secondary);
        CLAY({.layout = {.sizing = {.width = CLAY_SIZING_GROW(0)}, .childGap = 8, .padding = {0, 0, 2, 0}}, .clip = {.horizontal = true}}) {
            line(f->line > 0 ? F("%s:%d", f->file, f->line) : S(f->file), FONT_MONO, 11, over ? th.accent : th.muted);
            if (f->still_open) {
                line(S("still open"), FONT_TEXT, 11, th.warning);
            }
        }
    }
}

static void findings_view(const Pr *p, const Review *r)
{
    for (int c = 0; c < (int)(sizeof CATEGORIES / sizeof CATEGORIES[0]); c++) {
        int heading = 0;
        for (int i = 0; i < r->finding_count; i++) {
            if (strcmp(r->findings[i].category, CATEGORIES[c][0])) {
                continue;
            }
            if (!heading) {
                heading = 1;
                CLAY({.layout = {.padding = {8, 8, 14, 4}}}) {
                    line(S(CATEGORIES[c][1]), FONT_TEXT, 11, th.muted);
                }
            }
            finding(p, &r->findings[i], i);
        }
    }
}

static void actions(const Pr *p)
{
    const Review *done = pr_last_done(p);
    PrRef ref = pr_ref(p);
    CLAY({.layout = {.childGap = 6, .padding = {8, 0, 0, 0}}}) {
        if (status_running(p->status)) {
            text_button(CLAY_ID("cancel"), HIT_CANCEL, ref, "Cancel review", 0);
        } else {
            text_button(CLAY_ID("review"), HIT_REVIEW, ref, p->review_count ? "Re-review" : "Review", 0);
        }
        if (done && done->finding_count > 0) {
            text_button(CLAY_ID("nudge"), HIT_NUDGE, ref, done->nudged ? "Nudge again" : "Nudge", 0);
        }
        if (done && !*done->comment_url) {
            text_button(CLAY_ID("post"), HIT_POST, ref, "Post comment", 0);
        }
    }
}

static void rounds(const Pr *p, const Review *r)
{
    CLAY({.layout = {.sizing = {.width = CLAY_SIZING_GROW(0)}, .childGap = 2, .childAlignment = {.y = CLAY_ALIGN_Y_CENTER}, .padding = {4, 8, 0, 0}}}) {
        for (int i = 0; i < p->review_count; i++) {
            const Review *rv = &p->reviews[i];
            Clay_Color color = !strcmp(rv->status, "failed") ? th.danger : status_running(rv->status) ? th.pro : CLEAR;
            pill(CLAY_IDI("round", i), HIT_ROUND, i, pr_ref(p), F("Round %d", rv->round), rv == r, color);
        }
        spacer();
        if (*r->label) {
            line(S(r->label), FONT_TEXT, 12, th.muted);
        }
    }
    if (*r->nudge_error) {
        CLAY({.layout = {.sizing = {.width = CLAY_SIZING_GROW(0)}, .padding = {8, 8, 10, 0}}}) {
            text(F("Nudge failed: %s", r->nudge_error), FONT_TEXT, 12, th.danger);
        }
    }
}

static void body(const Pr *p, const Review *r)
{
    if (!r) {
        message(S("Not reviewed yet. Drag it into Reviewing, or press Review."), th.muted);
    } else if (status_running(r->status)) {
        log_view();
    } else if (!strcmp(r->status, "failed")) {
        message(S(*r->error ? r->error : "The review failed."), th.danger);
    } else if (r->finding_count == 0) {
        message(S("No findings."), th.success);
    } else {
        findings_view(p, r);
    }
}

static void content(const Pr *p)
{
    const Review *r = app_review(p);
    PrRef ref = pr_ref(p);
    CLAY({.layout = {.sizing = {CLAY_SIZING_FIXED(PANEL_WIDTH), CLAY_SIZING_GROW(0)}, .layoutDirection = CLAY_TOP_TO_BOTTOM, .padding = {16, 14, 12, 0}}}) {
        CLAY({.layout = {.sizing = {.width = CLAY_SIZING_GROW(0)}, .childAlignment = {.y = CLAY_ALIGN_Y_CENTER}, .padding = {8, 0, 0, 0}}}) {
            line(F("%s #%d", p->name, p->number), FONT_TEXT, 12, th.muted);
            spacer();
            icon_button(CLAY_ID("open"), HIT_OPEN, 0, ref, ICON_OPEN);
            icon_button(CLAY_ID("close"), HIT_CLOSE, 0, ref, ICON_CLOSE);
        }
        CLAY({.layout = {.sizing = {.width = CLAY_SIZING_GROW(0)}, .padding = {8, 16, 2, 0}}}) {
            text(S(p->title), FONT_MEDIUM, 15, th.text);
        }
        gap(14);
        actions(p);
        gap(18);
        if (r) {
            rounds(p, r);
        }
        gap(6);
        CLAY({.id = CLAY_ID("findings"), .layout = {.sizing = {CLAY_SIZING_GROW(0), CLAY_SIZING_GROW(0)}, .layoutDirection = CLAY_TOP_TO_BOTTOM,
            .padding = {0, 4, 4, 24}, .childGap = 2}, .clip = {.vertical = true, .childOffset = Clay_GetScrollOffset()}}) {
            body(p, r);
        }
    }
}

void panel_view(float open)
{
    const Pr *p = app_pr(app.selection.pr);
    float eased = 1.0f - (1.0f - open) * (1.0f - open) * (1.0f - open);
    CLAY({.id = CLAY_ID("panel"), .layout = {.sizing = {CLAY_SIZING_FIXED(PANEL_WIDTH * eased), CLAY_SIZING_GROW(0)}},
        .clip = {.horizontal = true}, .backgroundColor = th.panel, .border = {.color = th.border, .width = {.left = 1}}}) {
        if (p) {
            content(p);
        }
    }
}
