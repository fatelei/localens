//go:build darwin

package desktop

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static void localensConfigureWindowAppearance(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        id delegate = [NSApp delegate];
        if (![delegate respondsToSelector:@selector(mainWindow)]) return;
        NSWindow *window = [delegate valueForKey:@"mainWindow"];
        if (window == nil) return;
        // Select Wails' actual window; its localized title can change at runtime.
        [window setOpaque:NO];
        [window setBackgroundColor:[NSColor clearColor]];
        [window invalidateShadow];
    });
}
*/
import "C"

func ConfigureWindowAppearance() {
	C.localensConfigureWindowAppearance()
}
