#ifndef PRMAX_APP_H
#define PRMAX_APP_H

#include "model.h"
#include "platform/net.h"

#define REPO_MAX 16
#define LOG_MAX 300
#define OPTION_MAX 32

enum {
    COLUMN_TODO = 0,
    COLUMN_REVIEWING,
    COLUMN_ISSUES,
    COLUMN_PASSED,
    COLUMN_COUNT
};

enum {
    LOG_INFO = 0,
    LOG_TOOL,
    LOG_TEXT,
    LOG_ERROR,
    LOG_DONE
};

enum {
    ROW_PROVIDER = 0,
    ROW_MODEL,
    ROW_EFFORT,
    ROW_ACCESS,
    ROW_SCOPE,
    ROW_NUDGE,
    ROW_COUNT
};

typedef struct PrRef {
    uint32_t repo;
    int number;
} PrRef;

typedef struct Row {
    const char *labels[OPTION_MAX];
    int count;
    int selected;
} Row;

typedef struct App {
    State state;
    Providers providers;
    int connected;
    int ever_connected;
    struct {
        char name[REPO_MAX][64];
        int hidden[REPO_MAX];
        int count;
    } repos;
    struct {
        PrRef pr;
        int round;
        int open;
    } selection;
    struct {
        int open;
        PrRef pr;
        int value[ROW_COUNT];
        Plan plan;
        unsigned serial;
    } dialog;
    struct {
        char review[96];
        unsigned serial;
        int count;
        char *lines[LOG_MAX];
        int kinds[LOG_MAX];
    } logs;
    char toast[256];
    double toast_until;
} App;

extern App app;

void app_init(void);
void app_result(NetResult *result);
void app_event(NetEvent *event);

PrRef pr_ref(const Pr *p);
int pr_is(const Pr *p, PrRef ref);
Pr *app_pr(PrRef ref);
int pr_column(const Pr *p);
int status_running(const char *status);
int app_visible(const Pr *p);
void app_toggle_repo(int index);

const Pr *app_selected(void);
const Review *app_review(const Pr *p);
void app_select(const Pr *p);
void app_close(void);
void app_pick_round(int index);

void app_action(const Pr *p, const char *name);
void app_open_finding(const Pr *p, int index);
void app_toast(const char *message);
int app_can_drop(const Pr *p, int column);
void app_drop(const Pr *p, int column, int quick);

void dialog_open(const Pr *p);
void dialog_close(void);
void dialog_submit(void);
int dialog_rereview(void);
void dialog_row(int row, Row *out);
void dialog_choose(int row, int index);
void dialog_step(int row, int delta);

#endif
