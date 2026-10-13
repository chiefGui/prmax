#ifndef PRMAX_VIEWS_H
#define PRMAX_VIEWS_H

#include "kit.h"

#define PANEL_WIDTH 440.0f

enum {
    HIT_CARD = 1,
    HIT_REPO,
    HIT_ROUND,
    HIT_FINDING,
    HIT_OPEN,
    HIT_CLOSE,
    HIT_REVIEW,
    HIT_CANCEL,
    HIT_NUDGE,
    HIT_POST,
    HIT_DISMISS,
    HIT_BLOCK,
    HIT_OPTION,
    HIT_PREVIOUS,
    HIT_NEXT,
    HIT_SUBMIT
};

typedef struct Lift {
    const Pr *pr;
    float x, y, width;
} Lift;

void topbar_view(void);
void board_view(const Lift *lift);
int board_column_at(void);
void lift_view(const Lift *lift);
void panel_view(float open);
void dialog_view(float width, float height);
void toast_view(void);

#endif
