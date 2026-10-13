#include "views.h"

#include <string.h>

#define DIALOG_WIDTH 440.0f
#define DIALOG_PADDING 20
#define LABEL_WIDTH 74.0f
#define PILL_PADDING 18.0f
#define ICON_PREVIOUS "\xEE\x9D\xAB"
#define ICON_NEXT "\xEE\x9D\xAC"

static const char *ROW_NAMES[ROW_COUNT] = {"Provider", "Model", "Effort", "Access", "Scope", "Nudge"};

static int fits(const Row *r)
{
    float total = 0.0f;
    for (int i = 0; i < r->count; i++) {
        total += gfx_measure(r->labels[i], (int)strlen(r->labels[i]), FONT_TEXT, 12.0f) + PILL_PADDING;
    }
    return total <= DIALOG_WIDTH - 2 * DIALOG_PADDING - LABEL_WIDTH;
}

static void stepper(int row, const Row *r, PrRef pr)
{
    icon_button(CLAY_IDI("previous", row), HIT_PREVIOUS, row, pr, ICON_PREVIOUS);
    CLAY({.layout = {.padding = {9, 9, 4, 4}}, .cornerRadius = CLAY_CORNER_RADIUS(12), .backgroundColor = th.column}) {
        line(S(r->labels[r->selected]), FONT_TEXT, 12, th.text);
    }
    icon_button(CLAY_IDI("next", row), HIT_NEXT, row, pr, ICON_NEXT);
}

static void option_row(int row, PrRef pr)
{
    Row r;
    dialog_row(row, &r);
    CLAY({.layout = {.sizing = {.width = CLAY_SIZING_GROW(0)}, .childGap = 2, .childAlignment = {.y = CLAY_ALIGN_Y_CENTER}, .padding = {0, 0, 3, 3}}}) {
        CLAY({.layout = {.sizing = {.width = CLAY_SIZING_FIXED(LABEL_WIDTH)}}}) {
            line(S(ROW_NAMES[row]), FONT_TEXT, 12, th.muted);
        }
        if (r.count == 0) {
            line(S("none available"), FONT_TEXT, 12, th.muted);
        } else if (fits(&r)) {
            for (int i = 0; i < r.count; i++) {
                pill(CLAY_IDI("option", row * OPTION_MAX + i), HIT_OPTION, row * OPTION_MAX + i, pr, S(r.labels[i]), i == r.selected, CLEAR);
            }
        } else {
            stepper(row, &r, pr);
        }
    }
}

static void plan_line(void)
{
    const Plan *pl = &app.dialog.plan;
    if (!pl->loaded) {
        line(S("Checking what changed…"), FONT_TEXT, 12, th.muted);
    } else if (pl->error[0]) {
        text(S(pl->error), FONT_TEXT, 12, th.danger);
    } else if (pl->unchanged) {
        text(F("Nothing changed since round %d.", pl->since_round), FONT_TEXT, 12, th.muted);
        if (app.dialog.value[ROW_SCOPE] == 0) {
            text(S("Set scope to whole PR to review it again."), FONT_TEXT, 12, th.warning);
        }
    } else if (pl->since) {
        text(F("%d new %s since round %d", pl->commits, pl->commits == 1 ? "commit" : "commits", pl->since_round), FONT_TEXT, 12, th.muted);
    } else if (pl->rewritten) {
        text(S("History was rewritten, so this reviews the whole PR."), FONT_TEXT, 12, th.muted);
    } else {
        line(pl->round > 1 ? F("Round %d", pl->round) : S("First review"), FONT_TEXT, 12, th.muted);
    }
}

void dialog_view(float width, float height)
{
    const Pr *p = app_pr(app.dialog.pr);
    if (!p) {
        return;
    }
    PrRef pr = pr_ref(p);
    Clay_ElementId scrim = CLAY_ID("scrim");
    hit(scrim, HIT_DISMISS, 0, pr);
    CLAY({.id = scrim, .layout = {.sizing = {CLAY_SIZING_FIXED(width), CLAY_SIZING_FIXED(height)},
        .childAlignment = {CLAY_ALIGN_X_CENTER, CLAY_ALIGN_Y_CENTER}}, .backgroundColor = th.overlay,
        .floating = {.attachTo = CLAY_ATTACH_TO_ROOT, .zIndex = 100, .pointerCaptureMode = CLAY_POINTER_CAPTURE_MODE_CAPTURE}}) {
        Clay_ElementId box = CLAY_ID("dialog");
        hit(box, HIT_BLOCK, 0, pr);
        CLAY({.id = box, .layout = {.sizing = {.width = CLAY_SIZING_FIXED(DIALOG_WIDTH)}, .layoutDirection = CLAY_TOP_TO_BOTTOM,
            .padding = CLAY_PADDING_ALL(DIALOG_PADDING)}, .backgroundColor = th.panel, .cornerRadius = CLAY_CORNER_RADIUS(12),
            .border = {.color = th.border_hover, .width = CLAY_BORDER_OUTSIDE(1)}, .userData = &kit_shadow}) {
            line(F("%s #%d", p->name, p->number), FONT_TEXT, 12, th.muted);
            gap(2);
            text(S(p->title), FONT_MEDIUM, 14, th.text);
            gap(4);
            plan_line();
            gap(14);
            for (int row = 0; row < ROW_COUNT; row++) {
                if (row != ROW_SCOPE || dialog_rereview()) {
                    option_row(row, pr);
                }
            }
            gap(18);
            CLAY({.layout = {.sizing = {.width = CLAY_SIZING_GROW(0)}, .childGap = 6}}) {
                spacer();
                text_button(CLAY_ID("dismiss"), HIT_DISMISS, pr, "Cancel", 0);
                text_button(CLAY_ID("submit"), HIT_SUBMIT, pr, "Review", 1);
            }
        }
    }
}
