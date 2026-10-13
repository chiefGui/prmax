#include "app.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "host.h"
#include "yyjson.h"

#define TOAST_SECONDS 4.0

enum {
    TAG_PROVIDERS = 1,
    TAG_PLAN,
    TAG_LOG,
    TAG_ACTION,
    TAG_REVIEW
};

static const char *ACCESS[] = {"full", "read-only"};
static const char *SCOPES[] = {"changes", "whole PR"};
static const char *NUDGES[] = {"off", "after posting"};

App app;

PrRef pr_ref(const Pr *p) { return (PrRef){p->repo_key, p->number}; }

int pr_is(const Pr *p, PrRef ref) { return p && p->repo_key == ref.repo && p->number == ref.number; }

Pr *app_pr(PrRef ref)
{
    for (int i = 0; i < app.state.count; i++) {
        if (pr_is(&app.state.prs[i], ref)) {
            return &app.state.prs[i];
        }
    }
    return 0;
}

int status_running(const char *status) { return !strcmp(status, "queued") || !strcmp(status, "reviewing"); }

int pr_column(const Pr *p)
{
    if (status_running(p->status)) {
        return COLUMN_REVIEWING;
    }
    if (!strcmp(p->status, "findings")) {
        return COLUMN_ISSUES;
    }
    if (!strcmp(p->status, "clean")) {
        return COLUMN_PASSED;
    }
    return COLUMN_TODO;
}

static int repo_index(const char *repo)
{
    for (int i = 0; i < app.repos.count; i++) {
        if (!strcmp(app.repos.name[i], repo)) {
            return i;
        }
    }
    return -1;
}

int app_visible(const Pr *p)
{
    int i = repo_index(p->repo);
    return i < 0 || !app.repos.hidden[i];
}

void app_toggle_repo(int index)
{
    if (index >= 0 && index < app.repos.count) {
        app.repos.hidden[index] = !app.repos.hidden[index];
    }
}

static void collect_repos(void)
{
    for (int i = 0; i < app.state.count; i++) {
        const char *repo = app.state.prs[i].repo;
        if (repo_index(repo) >= 0 || app.repos.count == REPO_MAX) {
            continue;
        }
        int at = app.repos.count++;
        while (at > 0 && strcmp(app.repos.name[at - 1], repo) > 0) {
            strcpy(app.repos.name[at], app.repos.name[at - 1]);
            app.repos.hidden[at] = app.repos.hidden[at - 1];
            at--;
        }
        snprintf(app.repos.name[at], sizeof app.repos.name[at], "%s", repo);
        app.repos.hidden[at] = 0;
    }
}

void app_toast(const char *message)
{
    snprintf(app.toast, sizeof app.toast, "%s", message);
    app.toast_until = host_now() + TOAST_SECONDS;
    host_wake((int)(TOAST_SECONDS * 1000) + 50);
    host_redraw();
}

static void first_line(const NetResult *r, char *out, size_t size, const char *fallback)
{
    const char *body = r->body ? r->body : "";
    int n = (int)strcspn(body, "\r\n");
    if (r->status == 0) {
        snprintf(out, size, "Couldn't reach the daemon.");
    } else if (n == 0) {
        snprintf(out, size, "%s", fallback);
    } else {
        snprintf(out, size, "%.*s", n, body);
    }
}

static void pr_path(char *out, size_t size, const Pr *p, const char *action)
{
    snprintf(out, size, "/api/prs/%s/%d/%s", p->repo, p->number, action);
}

void app_action(const Pr *p, const char *name)
{
    char path[256];
    pr_path(path, sizeof path, p, name);
    net_post(path, 0, TAG_ACTION, 0);
}

static void load_providers(void) { net_get("/api/providers", TAG_PROVIDERS, 0); }

const Pr *app_selected(void) { return app.selection.open ? app_pr(app.selection.pr) : 0; }

const Review *app_review(const Pr *p)
{
    if (!p || p->review_count == 0) {
        return 0;
    }
    int i = app.selection.round;
    return &p->reviews[i >= 0 && i < p->review_count ? i : p->review_count - 1];
}

static void logs_clear(void)
{
    for (int i = 0; i < app.logs.count; i++) {
        free(app.logs.lines[i]);
    }
    app.logs.count = 0;
}

static int log_kind(const char *kind)
{
    static const char *names[] = {"info", "tool", "text", "error", "done"};
    for (int i = 0; kind && i < (int)(sizeof names / sizeof names[0]); i++) {
        if (!strcmp(kind, names[i])) {
            return i;
        }
    }
    return LOG_INFO;
}

static void logs_add(yyjson_val *line)
{
    const char *text = yyjson_get_str(yyjson_obj_get(line, "text"));
    if (!text || !*text) {
        return;
    }
    if (app.logs.count == LOG_MAX) {
        free(app.logs.lines[0]);
        memmove(app.logs.lines, app.logs.lines + 1, sizeof app.logs.lines[0] * (LOG_MAX - 1));
        memmove(app.logs.kinds, app.logs.kinds + 1, sizeof app.logs.kinds[0] * (LOG_MAX - 1));
        app.logs.count--;
    }
    app.logs.lines[app.logs.count] = _strdup(text);
    app.logs.kinds[app.logs.count] = log_kind(yyjson_get_str(yyjson_obj_get(line, "kind")));
    app.logs.count++;
}

static void load_logs(void)
{
    const Review *r = app_review(app_selected());
    if (!r || !status_running(r->status) || !strcmp(app.logs.review, r->id)) {
        return;
    }
    logs_clear();
    snprintf(app.logs.review, sizeof app.logs.review, "%s", r->id);
    app.logs.serial++;
    char path[160];
    snprintf(path, sizeof path, "/api/reviews/%s/log", r->id);
    net_get(path, TAG_LOG, app.logs.serial);
}

static void mark_seen(void)
{
    Pr *p = app_pr(app.selection.pr);
    if (app.selection.open && p && p->unread) {
        p->unread = 0;
        app_action(p, "seen");
    }
}

void app_select(const Pr *p)
{
    if (app.selection.open && pr_is(p, app.selection.pr)) {
        app_close();
        return;
    }
    app.selection.pr = pr_ref(p);
    app.selection.round = -1;
    app.selection.open = 1;
    app.logs.review[0] = 0;
    load_logs();
    mark_seen();
}

void app_close(void) { app.selection.open = 0; }

void app_pick_round(int index)
{
    const Pr *p = app_selected();
    app.selection.round = p && index == p->review_count - 1 ? -1 : index;
    load_logs();
}

void app_open_finding(const Pr *p, int index)
{
    const Review *r = app_review(p);
    if (!r || index >= r->finding_count) {
        return;
    }
    const Finding *f = &r->findings[index];
    char url[1024];
    if (f->line > 0) {
        snprintf(url, sizeof url, "https://github.com/%s/blob/%s/%s#L%d", p->repo, r->head_sha, f->file, f->line);
    } else {
        snprintf(url, sizeof url, "https://github.com/%s/blob/%s/%s", p->repo, r->head_sha, f->file);
    }
    host_open(url);
}

int app_can_drop(const Pr *p, int column)
{
    int from = pr_column(p);
    return column == COLUMN_REVIEWING ? from != COLUMN_REVIEWING : from == COLUMN_REVIEWING && column == COLUMN_TODO;
}

static const Provider *dialog_provider(void)
{
    int i = app.dialog.value[ROW_PROVIDER];
    return i < app.providers.count ? &app.providers.list[i] : 0;
}

static const Model *dialog_model(void)
{
    const Provider *pv = dialog_provider();
    int i = app.dialog.value[ROW_MODEL];
    return pv && i < pv->model_count ? &pv->models[i] : 0;
}

static int index_of(const char **list, int n, const char *value)
{
    for (int i = 0; value && i < n; i++) {
        if (!strcmp(list[i], value)) {
            return i;
        }
    }
    return 0;
}

static void sync_effort(void)
{
    const Provider *pv = dialog_provider();
    const Model *m = dialog_model();
    app.dialog.value[ROW_EFFORT] = 0;
    if (!pv || !m) {
        return;
    }
    const char *saved = providers_effort(&app.providers, pv->id);
    const char *want = saved && *saved ? saved : m->default_effort;
    app.dialog.value[ROW_EFFORT] = index_of(m->efforts, m->effort_count, want);
}

static void sync_model(void)
{
    const Provider *pv = dialog_provider();
    app.dialog.value[ROW_MODEL] = 0;
    if (pv) {
        const char *saved = providers_model(&app.providers, pv->id);
        const char *want = saved && *saved ? saved : pv->default_model;
        for (int i = 0; i < pv->model_count; i++) {
            if (!strcmp(pv->models[i].id, want)) {
                app.dialog.value[ROW_MODEL] = i;
            }
        }
    }
    sync_effort();
}

static void dialog_defaults(const Pr *p)
{
    const Providers *pr = &app.providers;
    app.dialog.pr = pr_ref(p);
    app.dialog.value[ROW_PROVIDER] = 0;
    for (int i = 0; i < pr->count; i++) {
        if (!strcmp(pr->list[i].id, pr->provider)) {
            app.dialog.value[ROW_PROVIDER] = i;
        }
    }
    sync_model();
    app.dialog.value[ROW_ACCESS] = index_of(ACCESS, 2, pr->access);
    app.dialog.value[ROW_SCOPE] = index_of(SCOPES, 2, pr->scope);
    app.dialog.value[ROW_NUDGE] = pr->nudge;
}

static void submit(const Pr *p)
{
    const Provider *pv = dialog_provider();
    const Model *m = dialog_model();
    int effort = app.dialog.value[ROW_EFFORT];
    if (!pv || !m) {
        app_toast("No models available.");
        return;
    }
    char body[512], path[256];
    snprintf(body, sizeof body,
        "{\"provider\":\"%s\",\"model\":\"%s\",\"effort\":\"%s\",\"access\":\"%s\",\"scope\":\"%s\",\"nudge\":%s}",
        pv->id, m->id, effort < m->effort_count ? m->efforts[effort] : "", ACCESS[app.dialog.value[ROW_ACCESS]],
        SCOPES[app.dialog.value[ROW_SCOPE]], app.dialog.value[ROW_NUDGE] ? "true" : "false");
    pr_path(path, sizeof path, p, "review");
    net_post(path, body, TAG_REVIEW, 0);
    app.dialog.open = 0;
}

void dialog_open(const Pr *p)
{
    if (!app.providers.count) {
        app_toast("Loading providers. Try again in a moment.");
        load_providers();
        return;
    }
    dialog_defaults(p);
    memset(&app.dialog.plan, 0, sizeof app.dialog.plan);
    app.dialog.open = 1;
    app.dialog.serial++;
    char path[256];
    pr_path(path, sizeof path, p, "plan");
    net_get(path, TAG_PLAN, app.dialog.serial);
}

void dialog_close(void) { app.dialog.open = 0; }

void dialog_submit(void)
{
    const Pr *p = app_pr(app.dialog.pr);
    if (app.dialog.open && p) {
        submit(p);
    }
}

void app_drop(const Pr *p, int column, int quick)
{
    if (!app_can_drop(p, column)) {
        return;
    }
    if (column == COLUMN_TODO) {
        app_action(p, "cancel");
    } else if (quick && app.providers.count) {
        dialog_defaults(p);
        submit(p);
    } else {
        dialog_open(p);
    }
}

int dialog_rereview(void) { return app.dialog.plan.loaded && (app.dialog.plan.since || app.dialog.plan.unchanged); }

static void fill(Row *out, const char **labels, int count)
{
    out->count = count < OPTION_MAX ? count : OPTION_MAX;
    for (int i = 0; i < out->count; i++) {
        out->labels[i] = labels[i];
    }
}

void dialog_row(int row, Row *out)
{
    const Provider *pv = dialog_provider();
    const Model *m = dialog_model();
    out->count = 0;
    out->selected = app.dialog.value[row];
    switch (row) {
    case ROW_PROVIDER:
        out->count = app.providers.count;
        for (int i = 0; i < out->count; i++) {
            out->labels[i] = app.providers.list[i].label;
        }
        break;
    case ROW_MODEL:
        out->count = pv ? (pv->model_count < OPTION_MAX ? pv->model_count : OPTION_MAX) : 0;
        for (int i = 0; i < out->count; i++) {
            out->labels[i] = pv->models[i].label;
        }
        break;
    case ROW_EFFORT:
        if (m) {
            fill(out, m->efforts, m->effort_count);
        }
        break;
    case ROW_ACCESS:
        fill(out, ACCESS, 2);
        break;
    case ROW_SCOPE:
        fill(out, SCOPES, 2);
        break;
    case ROW_NUDGE:
        fill(out, NUDGES, 2);
        break;
    }
}

void dialog_choose(int row, int index)
{
    Row r;
    dialog_row(row, &r);
    if (index < 0 || index >= r.count) {
        return;
    }
    app.dialog.value[row] = index;
    if (row == ROW_PROVIDER) {
        sync_model();
    } else if (row == ROW_MODEL) {
        sync_effort();
    }
}

void dialog_step(int row, int delta)
{
    Row r;
    dialog_row(row, &r);
    if (r.count) {
        dialog_choose(row, (r.selected + delta + r.count) % r.count);
    }
}

void app_init(void) { load_providers(); }

void app_event(NetEvent *e)
{
    if (e->type == EVENT_STATE) {
        if (!state_parse(&app.state, e->data, e->length)) {
            return;
        }
        app.connected = app.ever_connected = 1;
        collect_repos();
        if (!app_pr(app.selection.pr)) {
            app_close();
        }
        if (app.dialog.open && !app_pr(app.dialog.pr)) {
            dialog_close();
        }
        if (!app.providers.count) {
            load_providers();
        }
        load_logs();
        mark_seen();
    } else if (e->type == EVENT_LOG) {
        yyjson_doc *doc = yyjson_read(e->data, e->length, 0);
        yyjson_val *root = yyjson_doc_get_root(doc);
        const char *id = yyjson_get_str(yyjson_obj_get(root, "reviewId"));
        if (id && !strcmp(id, app.logs.review)) {
            logs_add(root);
        }
        yyjson_doc_free(doc);
    } else if (e->type == EVENT_OFFLINE) {
        app.connected = 0;
    }
}

void app_result(NetResult *r)
{
    int ok = r->status >= 200 && r->status < 300;
    char message[256];
    switch (r->tag) {
    case TAG_PROVIDERS:
        if (ok) {
            providers_parse(&app.providers, r->body, r->length);
        }
        break;
    case TAG_PLAN:
        if (app.dialog.open && r->context == app.dialog.serial) {
            if (ok) {
                plan_parse(&app.dialog.plan, r->body, r->length);
            } else {
                memset(&app.dialog.plan, 0, sizeof app.dialog.plan);
                app.dialog.plan.loaded = 1;
                first_line(r, app.dialog.plan.error, sizeof app.dialog.plan.error, "Couldn't check what changed.");
            }
        }
        break;
    case TAG_LOG:
        if (ok && r->context == app.logs.serial) {
            yyjson_doc *doc = yyjson_read(r->body, r->length, 0);
            size_t i, max;
            yyjson_val *line;
            logs_clear();
            yyjson_arr_foreach(yyjson_doc_get_root(doc), i, max, line) {
                logs_add(line);
            }
            yyjson_doc_free(doc);
        }
        break;
    case TAG_ACTION:
        if (!ok) {
            first_line(r, message, sizeof message, "That didn't work.");
            app_toast(message);
        }
        break;
    case TAG_REVIEW:
        if (ok) {
            load_providers();
        } else {
            first_line(r, message, sizeof message, "Couldn't start the review.");
            app_toast(message);
        }
        break;
    }
}
