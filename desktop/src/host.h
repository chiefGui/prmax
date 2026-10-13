#ifndef PRMAX_HOST_H
#define PRMAX_HOST_H

double host_now(void);
void host_redraw(void);
void host_wake(int ms);
void host_capture(int on);
void host_open(const char *url);

#endif
