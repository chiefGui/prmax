#include "model.h"

#include <stdlib.h>
#include <string.h>

#include "yyjson.h"

static const char *str(yyjson_val *obj, const char *key)
{
    const char *s = yyjson_get_str(yyjson_obj_get(obj, key));
    return s ? s : "";
}

static int num(yyjson_val *obj, const char *key)
{
    return (int)yyjson_get_int(yyjson_obj_get(obj, key));
}

static int boolean(yyjson_val *obj, const char *key)
{
    return yyjson_get_bool(yyjson_obj_get(obj, key)) ? 1 : 0;
}

static int newest_first(const void *a, const void *b)
{
    const Pr *x = a, *y = b;
    int c = strcmp(y->updated, x->updated);
    if (c != 0) {
        return c;
    }
    c = strcmp(x->repo, y->repo);
    return c != 0 ? c : x->number - y->number;
}

int state_parse(State *state, const char *json, int length)
{
    yyjson_doc *doc = yyjson_read(json, length, 0);
    if (!doc) {
        return 0;
    }
    yyjson_val *prs = yyjson_obj_get(yyjson_doc_get_root(doc), "prs");
    int count = (int)yyjson_arr_size(prs), reviews = 0, findings = 0;
    size_t i, max, j, jmax;
    yyjson_val *p, *r;
    yyjson_arr_foreach(prs, i, max, p) {
        yyjson_val *rs = yyjson_obj_get(p, "reviews");
        reviews += (int)yyjson_arr_size(rs);
        yyjson_arr_foreach(rs, j, jmax, r) {
            findings += (int)yyjson_arr_size(yyjson_obj_get(r, "findings"));
        }
    }
    State next = {0};
    next.doc = doc;
    next.count = count;
    next.prs = calloc(count ? count : 1, sizeof(Pr));
    next.review_pool = calloc(reviews ? reviews : 1, sizeof(Review));
    next.finding_pool = calloc(findings ? findings : 1, sizeof(Finding));
    Review *rp = next.review_pool;
    Finding *fp = next.finding_pool;
    int n = 0;
    yyjson_arr_foreach(prs, i, max, p) {
        Pr *pr = &next.prs[n++];
        pr->repo = str(p, "repo");
        const char *slash = strrchr(pr->repo, '/');
        pr->name = slash ? slash + 1 : pr->repo;
        pr->repo_key = repo_key(pr->repo);
        pr->title = str(p, "title");
        pr->url = str(p, "url");
        pr->status = str(p, "status");
        pr->updated = str(p, "updatedAt");
        pr->number = num(p, "number");
        pr->draft = boolean(p, "draft");
        pr->unread = num(p, "activity") > num(p, "seen");
        yyjson_val *rs = yyjson_obj_get(p, "reviews");
        pr->reviews = rp;
        yyjson_arr_foreach(rs, j, jmax, r) {
            Review *rv = rp++;
            rv->id = str(r, "id");
            rv->status = str(r, "status");
            rv->label = str(r, "modelLabel");
            rv->error = str(r, "error");
            rv->comment_url = str(r, "commentUrl");
            rv->nudge_error = str(r, "nudgeError");
            rv->head_sha = str(r, "headSha");
            rv->round = num(r, "round");
            rv->nudged = *str(r, "nudgedAt") != 0;
            if (rv->round == 0) {
                rv->round = (int)j + 1;
            }
            yyjson_val *fs = yyjson_obj_get(r, "findings");
            rv->findings = fp;
            size_t k, kmax;
            yyjson_val *f;
            yyjson_arr_foreach(fs, k, kmax, f) {
                Finding *fd = fp++;
                fd->category = str(f, "category");
                fd->file = str(f, "file");
                fd->title = str(f, "title");
                fd->text = str(f, "text");
                fd->line = num(f, "line");
                fd->still_open = boolean(f, "stillOpen");
            }
            rv->finding_count = (int)(fp - rv->findings);
            pr->review_count++;
        }
    }
    qsort(next.prs, count, sizeof(Pr), newest_first);
    state_free(state);
    *state = next;
    return 1;
}

void state_free(State *state)
{
    if (state->doc) {
        yyjson_doc_free(state->doc);
    }
    free(state->prs);
    free(state->review_pool);
    free(state->finding_pool);
    memset(state, 0, sizeof *state);
}

uint32_t repo_key(const char *repo)
{
    uint32_t h = 2166136261u;
    while (*repo) {
        h = (h ^ (unsigned char)*repo++) * 16777619u;
    }
    return h ? h : 1;
}

const Review *pr_last_done(const Pr *pr)
{
    for (int i = pr->review_count - 1; i >= 0; i--) {
        const char *s = pr->reviews[i].status;
        if (strcmp(s, "clean") == 0 || strcmp(s, "findings") == 0) {
            return &pr->reviews[i];
        }
    }
    return 0;
}

int providers_parse(Providers *providers, const char *json, int length)
{
    yyjson_doc *doc = yyjson_read(json, length, 0);
    if (!doc) {
        return 0;
    }
    providers_free(providers);
    yyjson_val *root = yyjson_doc_get_root(doc);
    providers->doc = doc;
    size_t i, max;
    yyjson_val *p;
    yyjson_arr_foreach(yyjson_obj_get(root, "list"), i, max, p) {
        if (providers->count == PROVIDER_MAX) {
            break;
        }
        Provider *pv = &providers->list[providers->count++];
        pv->id = str(p, "id");
        pv->label = str(p, "label");
        pv->default_model = str(p, "default");
        yyjson_val *ms = yyjson_obj_get(p, "models");
        pv->model_count = (int)yyjson_arr_size(ms);
        pv->models = calloc(pv->model_count ? pv->model_count : 1, sizeof(Model));
        size_t j, jmax;
        yyjson_val *m;
        yyjson_arr_foreach(ms, j, jmax, m) {
            Model *mo = &pv->models[j];
            mo->id = str(m, "id");
            mo->label = str(m, "label");
            mo->default_effort = str(m, "defaultEffort");
            yyjson_val *es = yyjson_obj_get(m, "efforts");
            mo->effort_count = (int)yyjson_arr_size(es);
            mo->efforts = calloc(mo->effort_count ? mo->effort_count : 1, sizeof(char *));
            size_t k, kmax;
            yyjson_val *e;
            yyjson_arr_foreach(es, k, kmax, e) {
                mo->efforts[k] = yyjson_get_str(e);
            }
        }
    }
    yyjson_val *choice = yyjson_obj_get(root, "choice");
    providers->provider = str(choice, "provider");
    providers->access = str(choice, "access");
    providers->scope = str(choice, "scope");
    providers->nudge = boolean(choice, "nudge");
    providers->models = yyjson_obj_get(choice, "models");
    providers->efforts = yyjson_obj_get(choice, "efforts");
    return 1;
}

void providers_free(Providers *providers)
{
    for (int i = 0; i < providers->count; i++) {
        for (int j = 0; j < providers->list[i].model_count; j++) {
            free(providers->list[i].models[j].efforts);
        }
        free(providers->list[i].models);
    }
    if (providers->doc) {
        yyjson_doc_free(providers->doc);
    }
    memset(providers, 0, sizeof *providers);
}

const char *providers_model(const Providers *providers, const char *provider)
{
    return yyjson_get_str(yyjson_obj_get(providers->models, provider));
}

const char *providers_effort(const Providers *providers, const char *provider)
{
    return yyjson_get_str(yyjson_obj_get(providers->efforts, provider));
}

void plan_parse(Plan *plan, const char *json, int length)
{
    memset(plan, 0, sizeof *plan);
    plan->loaded = 1;
    yyjson_doc *doc = yyjson_read(json, length, 0);
    if (!doc) {
        return;
    }
    yyjson_val *root = yyjson_doc_get_root(doc);
    plan->round = num(root, "round");
    plan->since = *str(root, "since") != 0;
    plan->since_round = num(root, "sinceRound");
    plan->commits = (int)yyjson_arr_size(yyjson_obj_get(root, "commits"));
    plan->rewritten = boolean(root, "rewritten");
    plan->unchanged = boolean(root, "unchanged");
    yyjson_doc_free(doc);
}
