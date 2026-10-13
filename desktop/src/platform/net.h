#ifndef PRMAX_NET_H
#define PRMAX_NET_H

#include <windows.h>
#include <stdint.h>

#define WM_NET_RESULT (WM_APP + 1)
#define WM_NET_EVENT (WM_APP + 2)

enum {
    EVENT_STATE = 1,
    EVENT_LOG,
    EVENT_OFFLINE
};

typedef struct NetResult {
    int tag;
    uintptr_t context;
    int status;
    char *body;
    int length;
} NetResult;

typedef struct NetEvent {
    int type;
    char *data;
    int length;
} NetEvent;

void net_init(HWND notify);
void net_get(const char *path, int tag, uintptr_t context);
void net_post(const char *path, const char *body, int tag, uintptr_t context);
void net_stream(void);
void net_free_result(NetResult *result);
void net_free_event(NetEvent *event);
int net_ping(void);

#endif
