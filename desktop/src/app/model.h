#ifndef PRMAX_MODEL_H
#define PRMAX_MODEL_H

#include <stdint.h>

#define PROVIDER_MAX 4

typedef struct Finding {
    const char *category;
    const char *file;
    const char *title;
    const char *text;
    int line;
    int still_open;
} Finding;

typedef struct Review {
    const char *id;
    const char *status;
    const char *label;
    const char *error;
    const char *comment_url;
    const char *nudge_error;
    const char *head_sha;
    int round;
    int nudged;
    int finding_count;
    Finding *findings;
} Review;

typedef struct Pr {
    const char *repo;
    const char *name;
    const char *title;
    const char *url;
    const char *status;
    const char *updated;
    uint32_t repo_key;
    int number;
    int draft;
    int unread;
    int review_count;
    Review *reviews;
} Pr;

typedef struct State {
    void *doc;
    int count;
    Pr *prs;
    Review *review_pool;
    Finding *finding_pool;
} State;

typedef struct Model {
    const char *id;
    const char *label;
    const char *default_effort;
    int effort_count;
    const char **efforts;
} Model;

typedef struct Provider {
    const char *id;
    const char *label;
    const char *default_model;
    int model_count;
    Model *models;
} Provider;

typedef struct Providers {
    void *doc;
    int count;
    Provider list[PROVIDER_MAX];
    const char *provider;
    const char *access;
    const char *scope;
    int nudge;
    void *models;
    void *efforts;
} Providers;

typedef struct Plan {
    int loaded;
    int round;
    int since_round;
    int commits;
    int rewritten;
    int unchanged;
    int since;
    char error[160];
} Plan;

int state_parse(State *state, const char *json, int length);
void state_free(State *state);
const Review *pr_last_done(const Pr *pr);
uint32_t repo_key(const char *repo);
int providers_parse(Providers *providers, const char *json, int length);
void providers_free(Providers *providers);
const char *providers_model(const Providers *providers, const char *provider);
const char *providers_effort(const Providers *providers, const char *provider);
void plan_parse(Plan *plan, const char *json, int length);

#endif
