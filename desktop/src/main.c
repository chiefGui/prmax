#include <windows.h>
#include <windowsx.h>
#include <dwmapi.h>
#include <shellapi.h>

#include "app/app.h"
#include "host.h"
#include "platform/gfx.h"
#include "platform/net.h"
#include "ui/ui.h"

#define TIMER_ANIMATE 1
#define TIMER_WAKE 2
#define AMBIENT_MS 33
#define DWMWA_DARK_MODE 20

static HWND window;
static float scale = 1.0f;
static int ambient;

double host_now(void)
{
    static LARGE_INTEGER freq;
    LARGE_INTEGER t;
    if (!freq.QuadPart) {
        QueryPerformanceFrequency(&freq);
    }
    QueryPerformanceCounter(&t);
    return (double)t.QuadPart / (double)freq.QuadPart;
}

void host_redraw(void) { InvalidateRect(window, 0, FALSE); }

void host_wake(int ms) { SetTimer(window, TIMER_WAKE, (UINT)ms, 0); }

void host_capture(int on)
{
    if (on) {
        SetCapture(window);
    } else if (GetCapture() == window) {
        ReleaseCapture();
    }
}

void host_open(const char *url)
{
    wchar_t wide[1024];
    if (url && *url && MultiByteToWideChar(CP_UTF8, 0, url, -1, wide, 1024)) {
        ShellExecuteW(0, L"open", wide, 0, 0, SW_SHOWNORMAL);
    }
}

static int light_theme(void)
{
    DWORD value = 0, size = sizeof value;
    return RegGetValueW(HKEY_CURRENT_USER, L"Software\\Microsoft\\Windows\\CurrentVersion\\Themes\\Personalize",
               L"AppsUseLightTheme", RRF_RT_REG_DWORD, 0, &value, &size) == ERROR_SUCCESS && value != 0;
}

static void apply_theme(void)
{
    BOOL dark = !light_theme();
    DwmSetWindowAttribute(window, DWMWA_DARK_MODE, &dark, sizeof dark);
    ui_theme(!dark);
}

static void schedule(void)
{
    int mode = ui_animating();
    if (mode == ANIMATE_SMOOTH) {
        host_redraw();
    }
    if ((mode == ANIMATE_AMBIENT) != ambient) {
        ambient = mode == ANIMATE_AMBIENT;
        if (ambient) {
            SetTimer(window, TIMER_ANIMATE, AMBIENT_MS, 0);
        } else {
            KillTimer(window, TIMER_ANIMATE);
        }
    }
}

static void start_server(void)
{
    wchar_t path[MAX_PATH], cmd[MAX_PATH + 24];
    DWORD n = GetModuleFileNameW(0, path, MAX_PATH);
    while (n > 0 && path[n - 1] != L'\\') {
        n--;
    }
    path[n] = 0;
    wsprintfW(cmd, L"\"%sprmax-server.exe\"", path);
    STARTUPINFOW si = {sizeof si};
    PROCESS_INFORMATION pi;
    DWORD flags = CREATE_NO_WINDOW | CREATE_NEW_PROCESS_GROUP;
    if (CreateProcessW(0, cmd, 0, 0, FALSE, flags | CREATE_BREAKAWAY_FROM_JOB, 0, 0, &si, &pi) ||
        CreateProcessW(0, cmd, 0, 0, FALSE, flags, 0, 0, &si, &pi)) {
        CloseHandle(pi.hThread);
        CloseHandle(pi.hProcess);
    }
}

static void resize(void)
{
    RECT rc;
    GetClientRect(window, &rc);
    float dpi = (float)GetDpiForWindow(window);
    scale = dpi / 96.0f;
    gfx_resize((UINT)rc.right, (UINT)rc.bottom, dpi);
    ui_resize((float)rc.right / scale, (float)rc.bottom / scale);
}

static float px(LPARAM lp, int y) { return (float)(y ? GET_Y_LPARAM(lp) : GET_X_LPARAM(lp)) / scale; }

static LRESULT CALLBACK proc(HWND hwnd, UINT msg, WPARAM wp, LPARAM lp)
{
    switch (msg) {
    case WM_CREATE:
        window = hwnd;
        if (!gfx_init(hwnd)) {
            return -1;
        }
        net_init(hwnd);
        ui_init();
        apply_theme();
        resize();
        net_stream(start_server);
        return 0;
    case WM_SIZE:
        resize();
        ui_paint();
        ValidateRect(hwnd, 0);
        return 0;
    case WM_DPICHANGED: {
        RECT *r = (RECT *)lp;
        SetWindowPos(hwnd, 0, r->left, r->top, r->right - r->left, r->bottom - r->top, SWP_NOZORDER | SWP_NOACTIVATE);
        return 0;
    }
    case WM_GETMINMAXINFO: {
        MINMAXINFO *m = (MINMAXINFO *)lp;
        UINT dpi = GetDpiForWindow(hwnd);
        m->ptMinTrackSize.x = MulDiv(760, dpi, 96);
        m->ptMinTrackSize.y = MulDiv(480, dpi, 96);
        return 0;
    }
    case WM_ERASEBKGND:
        return 1;
    case WM_PAINT:
        ui_paint();
        ValidateRect(hwnd, 0);
        schedule();
        return 0;
    case WM_TIMER:
        if (wp == TIMER_WAKE) {
            KillTimer(hwnd, TIMER_WAKE);
        }
        host_redraw();
        return 0;
    case WM_MOUSEMOVE: {
        TRACKMOUSEEVENT t = {sizeof t, TME_LEAVE, hwnd, 0};
        TrackMouseEvent(&t);
        ui_pointer_move(px(lp, 0), px(lp, 1));
        return 0;
    }
    case WM_MOUSELEAVE:
        ui_pointer_leave();
        return 0;
    case WM_LBUTTONDOWN:
        ui_pointer_down(px(lp, 0), px(lp, 1), (wp & MK_SHIFT) != 0);
        return 0;
    case WM_LBUTTONUP:
        ui_pointer_up(px(lp, 0), px(lp, 1), (wp & MK_SHIFT) != 0);
        return 0;
    case WM_CAPTURECHANGED:
        if ((HWND)lp != hwnd) {
            ui_capture_lost();
        }
        return 0;
    case WM_MOUSEWHEEL:
        ui_wheel((float)GET_WHEEL_DELTA_WPARAM(wp));
        return 0;
    case WM_KEYDOWN:
        if (wp == VK_ESCAPE || wp == VK_RETURN) {
            ui_key(wp == VK_ESCAPE ? KEY_ESCAPE : KEY_ENTER);
        }
        return 0;
    case WM_SETTINGCHANGE:
        if (lp && lstrcmpW((const wchar_t *)lp, L"ImmersiveColorSet") == 0) {
            apply_theme();
        }
        return 0;
    case WM_NET_RESULT:
        app_result((NetResult *)lp);
        net_free_result((NetResult *)lp);
        host_redraw();
        return 0;
    case WM_NET_EVENT:
        app_event((NetEvent *)lp);
        net_free_event((NetEvent *)lp);
        host_redraw();
        return 0;
    case WM_DESTROY:
        gfx_shutdown();
        PostQuitMessage(0);
        return 0;
    }
    return DefWindowProcW(hwnd, msg, wp, lp);
}

int WINAPI wWinMain(HINSTANCE inst, HINSTANCE prev, PWSTR cmdline, int show)
{
    (void)prev;
    (void)cmdline;
    SetProcessDpiAwarenessContext(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2);
    WNDCLASSEXW wc = {sizeof wc};
    wc.style = CS_HREDRAW | CS_VREDRAW;
    wc.lpfnWndProc = proc;
    wc.hInstance = inst;
    wc.hCursor = LoadCursor(0, IDC_ARROW);
    wc.hIcon = LoadIconW(inst, MAKEINTRESOURCEW(1));
    wc.hIconSm = (HICON)LoadImageW(inst, MAKEINTRESOURCEW(1), IMAGE_ICON, GetSystemMetrics(SM_CXSMICON), GetSystemMetrics(SM_CYSMICON), 0);
    wc.lpszClassName = L"prmax";
    RegisterClassExW(&wc);
    UINT dpi = GetDpiForSystem();
    HWND hwnd = CreateWindowExW(0, L"prmax", L"prmax", WS_OVERLAPPEDWINDOW, CW_USEDEFAULT, CW_USEDEFAULT,
        MulDiv(1360, dpi, 96), MulDiv(860, dpi, 96), 0, 0, inst, 0);
    if (!hwnd) {
        return 1;
    }
    ShowWindow(hwnd, show);
    MSG msg;
    while (GetMessageW(&msg, 0, 0, 0) > 0) {
        TranslateMessage(&msg);
        DispatchMessageW(&msg);
    }
    return 0;
}
