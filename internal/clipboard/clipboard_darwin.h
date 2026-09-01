#ifndef TAILBOARD_CLIPBOARD_DARWIN_H
#define TAILBOARD_CLIPBOARD_DARWIN_H

#include <stdint.h>

int64_t tailboard_pasteboard_change_count(void);
int tailboard_pasteboard_should_ignore(void);
char *tailboard_pasteboard_copy_text(void);
int tailboard_pasteboard_set_text(const char *text);
int tailboard_pasteboard_clear(void);

#endif
