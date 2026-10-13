#include "net.h"

#include <winhttp.h>
#include <process.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "yyjson.h"

#define RESTART_MS 5000

static HWND target;
static HINTERNET session;
static wchar_t host[64] = L"127.0.0.1";
static INTERNET_PORT port = 47821;
static void (*start_server)(void);

typedef struct Request {
    wchar_t method[8];
    wchar_t path[512];
    char *body;
    int tag;
    uintptr_t context;
} Request;

static void load_listen(void)
{
    char dir[MAX_PATH];
    char path[MAX_PATH];
    DWORD n = GetEnvironmentVariableA("PRMAX_HOME", dir, sizeof dir);
    if (n == 0 || n >= sizeof dir) {
        n = GetEnvironmentVariableA("LOCALAPPDATA", dir, sizeof dir);
        if (n == 0 || n >= sizeof dir) {
            return;
        }
        strncat(dir, "\\prmax", sizeof dir - strlen(dir) - 1);
    }
    snprintf(path, sizeof path, "%s\\config.json", dir);
    yyjson_doc *doc = yyjson_read_file(path, 0, 0, 0);
    if (!doc) {
        return;
    }
    const char *listen = yyjson_get_str(yyjson_obj_get(yyjson_doc_get_root(doc), "listen"));
    const char *colon = listen ? strrchr(listen, ':') : 0;
    if (colon && colon > listen && colon - listen < 63) {
        int len = MultiByteToWideChar(CP_UTF8, 0, listen, (int)(colon - listen), host, 63);
        host[len] = 0;
        port = (INTERNET_PORT)atoi(colon + 1);
    }
    yyjson_doc_free(doc);
}

void net_init(HWND notify)
{
    target = notify;
    load_listen();
    session = WinHttpOpen(L"prmax-desktop", WINHTTP_ACCESS_TYPE_NO_PROXY, WINHTTP_NO_PROXY_NAME, WINHTTP_NO_PROXY_BYPASS, 0);
}

static HINTERNET open_request(HINTERNET *connection, const wchar_t *method, const wchar_t *path)
{
    *connection = WinHttpConnect(session, host, port, 0);
    if (!*connection) {
        return 0;
    }
    HINTERNET req = WinHttpOpenRequest(*connection, method, path, 0, WINHTTP_NO_REFERER, WINHTTP_DEFAULT_ACCEPT_TYPES, 0);
    if (!req) {
        WinHttpCloseHandle(*connection);
        *connection = 0;
    }
    return req;
}

static int read_more(HINTERNET req, char **buf, int *len, int *cap)
{
    DWORD avail = 0, got = 0;
    if (!WinHttpQueryDataAvailable(req, &avail) || avail == 0) {
        return 0;
    }
    while (*len + (int)avail + 1 > *cap) {
        *cap *= 2;
        *buf = realloc(*buf, *cap);
    }
    if (!WinHttpReadData(req, *buf + *len, avail, &got)) {
        return 0;
    }
    *len += (int)got;
    (*buf)[*len] = 0;
    return (int)got;
}

static char *read_all(HINTERNET req, int *length)
{
    int cap = 8192;
    char *buf = malloc(cap);
    *length = 0;
    buf[0] = 0;
    while (read_more(req, &buf, length, &cap) > 0) {
    }
    return buf;
}

static int send_request(HINTERNET req, const char *body)
{
    DWORD n = body ? (DWORD)strlen(body) : 0;
    const wchar_t *headers = body ? L"Content-Type: application/json\r\n" : WINHTTP_NO_ADDITIONAL_HEADERS;
    if (!WinHttpSendRequest(req, headers, body ? (DWORD)-1L : 0, (void *)body, n, n, 0)) {
        return 0;
    }
    if (!WinHttpReceiveResponse(req, 0)) {
        return 0;
    }
    DWORD status = 0, size = sizeof status;
    WinHttpQueryHeaders(req, WINHTTP_QUERY_STATUS_CODE | WINHTTP_QUERY_FLAG_NUMBER, WINHTTP_HEADER_NAME_BY_INDEX, &status, &size, WINHTTP_NO_HEADER_INDEX);
    return (int)status;
}

static unsigned __stdcall request_thread(void *arg)
{
    Request *r = arg;
    NetResult *res = calloc(1, sizeof *res);
    res->tag = r->tag;
    res->context = r->context;
    HINTERNET conn = 0;
    HINTERNET req = open_request(&conn, r->method, r->path);
    if (req) {
        WinHttpSetTimeouts(req, 2000, 2000, 30000, 60000);
        res->status = send_request(req, r->body);
        if (res->status) {
            res->body = read_all(req, &res->length);
        }
        WinHttpCloseHandle(req);
        WinHttpCloseHandle(conn);
    }
    free(r->body);
    free(r);
    if (!PostMessageW(target, WM_NET_RESULT, 0, (LPARAM)res)) {
        net_free_result(res);
    }
    return 0;
}

static void start(const wchar_t *method, const char *path, const char *body, int tag, uintptr_t context)
{
    Request *r = calloc(1, sizeof *r);
    wcscpy(r->method, method);
    MultiByteToWideChar(CP_UTF8, 0, path, -1, r->path, 511);
    r->body = body ? _strdup(body) : 0;
    r->tag = tag;
    r->context = context;
    HANDLE t = (HANDLE)_beginthreadex(0, 0, request_thread, r, 0, 0);
    if (t) {
        CloseHandle(t);
    }
}

void net_get(const char *path, int tag, uintptr_t context) { start(L"GET", path, 0, tag, context); }

void net_post(const char *path, const char *body, int tag, uintptr_t context) { start(L"POST", path, body ? body : "", tag, context); }

void net_free_result(NetResult *result)
{
    free(result->body);
    free(result);
}

void net_free_event(NetEvent *event)
{
    free(event->data);
    free(event);
}

static void post_event(int type, const char *data, int length)
{
    NetEvent *e = calloc(1, sizeof *e);
    e->type = type;
    if (data) {
        e->data = malloc(length + 1);
        memcpy(e->data, data, length);
        e->data[length] = 0;
        e->length = length;
    }
    if (!PostMessageW(target, WM_NET_EVENT, 0, (LPARAM)e)) {
        net_free_event(e);
    }
}

static int starts(const char *s, int n, const char *prefix)
{
    int k = (int)strlen(prefix);
    return n >= k && !memcmp(s, prefix, k);
}

static int equals(const char *s, int n, const char *word) { return n == (int)strlen(word) && !memcmp(s, word, n); }

static void stream_once(void)
{
    HINTERNET conn = 0;
    HINTERNET req = open_request(&conn, L"GET", L"/api/events");
    if (!req) {
        return;
    }
    WinHttpSetTimeouts(req, 2000, 2000, 2000, 0);
    if (send_request(req, 0) == 200) {
        int cap = 1 << 16, len = 0, type = 0;
        char *buf = malloc(cap);
        while (read_more(req, &buf, &len, &cap) > 0) {
            char *line = buf;
            char *end;
            while ((end = memchr(line, '\n', len - (line - buf))) != 0) {
                int n = (int)(end - line);
                if (n > 0 && line[n - 1] == '\r') {
                    n--;
                }
                if (starts(line, n, "event: ")) {
                    type = equals(line + 7, n - 7, "state") ? EVENT_STATE : equals(line + 7, n - 7, "log") ? EVENT_LOG : 0;
                } else if (type && starts(line, n, "data: ")) {
                    post_event(type, line + 6, n - 6);
                }
                line = end + 1;
            }
            len -= (int)(line - buf);
            memmove(buf, line, len);
        }
        free(buf);
    }
    WinHttpCloseHandle(req);
    WinHttpCloseHandle(conn);
}

static int ping(void)
{
    HINTERNET conn = 0;
    HINTERNET req = open_request(&conn, L"GET", L"/api/state");
    if (!req) {
        return 0;
    }
    WinHttpSetTimeouts(req, 500, 500, 500, 1000);
    int ok = send_request(req, 0) == 200;
    WinHttpCloseHandle(req);
    WinHttpCloseHandle(conn);
    return ok;
}

static unsigned __stdcall stream_thread(void *arg)
{
    (void)arg;
    ULONGLONG started = 0;
    for (;;) {
        if (GetTickCount64() - started > RESTART_MS && !ping()) {
            start_server();
            started = GetTickCount64();
        }
        stream_once();
        post_event(EVENT_OFFLINE, 0, 0);
        Sleep(1000);
    }
}

void net_stream(void (*start)(void))
{
    start_server = start;
    HANDLE t = (HANDLE)_beginthreadex(0, 0, stream_thread, 0, 0, 0);
    if (t) {
        CloseHandle(t);
    }
}
