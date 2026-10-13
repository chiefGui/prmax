#ifndef PRMAX_UI_H
#define PRMAX_UI_H

enum {
    ANIMATE_NONE = 0,
    ANIMATE_AMBIENT,
    ANIMATE_SMOOTH
};

enum {
    KEY_ESCAPE = 1,
    KEY_ENTER
};

void ui_init(void);
void ui_theme(int light);
void ui_resize(float width, float height);
void ui_paint(void);
int ui_animating(void);
void ui_pointer_move(float x, float y);
void ui_pointer_leave(void);
void ui_pointer_down(float x, float y, int shift);
void ui_pointer_up(float x, float y, int shift);
void ui_capture_lost(void);
void ui_wheel(float delta);
void ui_key(int key);

#endif
