//go:build darwin

#import <AppKit/AppKit.h>
#import <Foundation/Foundation.h>

#include <stdlib.h>

#include "clipboard_darwin.h"

int64_t tailboard_pasteboard_change_count(void) {
    @autoreleasepool {
        return (int64_t)[[NSPasteboard generalPasteboard] changeCount];
    }
}

int tailboard_pasteboard_should_ignore(void) {
    @autoreleasepool {
        NSArray<NSPasteboardType> *types = [[NSPasteboard generalPasteboard] types];

        // These marker types are the established macOS convention used by
        // password managers, dictation tools, and other paste helpers to tell
        // clipboard-history software not to retain an ephemeral payload.
        // Wispr Flow marks the temporary text it pastes this way before
        // restoring the user's prior clipboard a fraction of a second later.
        return [types containsObject:@"org.nspasteboard.ConcealedType"]
            || [types containsObject:@"org.nspasteboard.TransientType"];
    }
}

char *tailboard_pasteboard_copy_text(void) {
    @autoreleasepool {
        NSString *text = [[NSPasteboard generalPasteboard] stringForType:NSPasteboardTypeString];
        const char *utf8 = [text UTF8String];
        return utf8 == NULL ? NULL : strdup(utf8);
    }
}

int tailboard_pasteboard_set_text(const char *text) {
    if (text == NULL) {
        return 0;
    }

    @autoreleasepool {
        NSString *value = [NSString stringWithUTF8String:text];
        NSPasteboard *pasteboard = [NSPasteboard generalPasteboard];
        [pasteboard clearContents];
        return [pasteboard setString:value forType:NSPasteboardTypeString] ? 1 : 0;
    }
}

int tailboard_pasteboard_clear(void) {
    @autoreleasepool {
        [[NSPasteboard generalPasteboard] clearContents];
        return 1;
    }
}
