//go:build darwin

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>
extern void localensStatusAction(int action);

@interface LocalensStatusTarget : NSObject
- (void)click:(id)sender;
- (void)selectAction:(NSMenuItem *)sender;
@end

// Add only the missing reopen callback; preserve Wails' existing delegate and lifecycle.
@interface LocalensDockActions : NSObject
- (BOOL)applicationShouldHandleReopen:(NSApplication *)application hasVisibleWindows:(BOOL)visible;
@end

@implementation LocalensDockActions
- (BOOL)applicationShouldHandleReopen:(NSApplication *)application hasVisibleWindows:(BOOL)visible {
    id delegate = [application delegate];
    NSWindow *window = nil;
    if ([delegate respondsToSelector:@selector(mainWindow)]) {
        window = [delegate valueForKey:@"mainWindow"];
    }
    [application unhide:nil];
    if (window != nil) {
        [window deminiaturize:nil];
        [window makeKeyAndOrderFront:nil];
    }
    [application activateIgnoringOtherApps:YES];
    // The buffered Go bridge focuses the input without blocking AppKit.
    localensStatusAction(0);
    return YES;
}
@end

static void localensInstallDockReopen(void) {
    id delegate = [NSApp delegate];
    if (delegate == nil) return;
    SEL selector = @selector(applicationShouldHandleReopen:hasVisibleWindows:);
    if ([delegate respondsToSelector:selector]) return;
    Method method = class_getInstanceMethod([LocalensDockActions class], selector);
    class_addMethod([delegate class], selector, method_getImplementation(method), method_getTypeEncoding(method));
}

static BOOL localensTrayEnglish = NO;
static NSStatusItem *localensStatusItem = nil;
static LocalensStatusTarget *localensStatusTarget = nil;

@implementation LocalensStatusTarget
- (void)selectAction:(NSMenuItem *)sender {
    localensStatusAction((int)[sender tag]);
}

- (void)click:(id)sender {
    if ([[NSApp currentEvent] type] != NSEventTypeRightMouseUp) {
        localensStatusAction(0);
        return;
    }
    NSMenu *menu = [[[NSMenu alloc] initWithTitle:@"拾物"] autorelease];
    NSArray *titles = localensTrayEnglish ? @[@"Show Search", @"Settings…", @"Quit Shiwa"] : @[@"显示搜索", @"设置…", @"退出拾物"];
    for (NSInteger i = 0; i < (NSInteger)[titles count]; i++) {
        if (i == 2) [menu addItem:[NSMenuItem separatorItem]];
        NSMenuItem *item = [[[NSMenuItem alloc] initWithTitle:titles[i]
            action:@selector(selectAction:) keyEquivalent:@""] autorelease];
        [item setTarget:self];
        [item setTag:i];
        [menu addItem:item];
    }
    [localensStatusItem popUpStatusItemMenu:menu];
}
@end

void localensInstallStatusItem(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        localensInstallDockReopen();
        if (localensStatusItem != nil) return;
        NSString *iconPath = [[NSBundle mainBundle] pathForResource:@"AppIcon" ofType:@"icns"];
        if (iconPath != nil) {
            NSImage *dockIcon = [[NSImage alloc] initWithContentsOfFile:iconPath];
            if (dockIcon != nil) [NSApp setApplicationIconImage:dockIcon];
            [dockIcon release];
        }
        localensStatusTarget = [[LocalensStatusTarget alloc] init];
        localensStatusItem = [[[NSStatusBar systemStatusBar]
            statusItemWithLength:NSSquareStatusItemLength] retain];
        NSStatusBarButton *button = [localensStatusItem button];
        NSImage *image = [NSImage imageWithSystemSymbolName:@"magnifyingglass"
            accessibilityDescription:@"拾物本地搜索"];
        [image setSize:NSMakeSize(18, 18)];
        [image setTemplate:YES];
        [button setImage:image];
        [button setToolTip:@"拾物 · 点击搜索，右键设置"];
        [button setAccessibilityLabel:@"拾物本地搜索"];
        [button setTarget:localensStatusTarget];
        [button setAction:@selector(click:)];
        [button sendActionOn:NSEventMaskLeftMouseUp | NSEventMaskRightMouseUp];
    });
}

void localensRemoveStatusItem(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (localensStatusItem == nil) return;
        [[NSStatusBar systemStatusBar] removeStatusItem:localensStatusItem];
        [localensStatusItem release];
        localensStatusItem = nil;
        [localensStatusTarget release];
        localensStatusTarget = nil;
    });
}

void localensSetTrayLanguage(int english) {
 dispatch_async(dispatch_get_main_queue(), ^{
  localensTrayEnglish = english != 0;
  if (localensStatusItem != nil) {
   [[localensStatusItem button] setToolTip:localensTrayEnglish ? @"Shiwa · Click to search, right-click for settings" : @"拾物 · 点击搜索，右键设置"];
   [[localensStatusItem button] setAccessibilityLabel:localensTrayEnglish ? @"Shiwa Search" : @"拾物本地搜索"];
  }
 });
}
