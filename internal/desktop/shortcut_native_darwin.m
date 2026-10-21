//go:build darwin

#import <Cocoa/Cocoa.h>
#import <Carbon/Carbon.h>
#include <stdint.h>
extern void localensNativeHotkeyEvent(uint32_t id, int pressed);
extern int localensNativeHotkeyAlive(uint32_t id);
extern void localensNativeHotkeyRegistered(uint32_t id, int status, uintptr_t reference);

static EventHandlerRef localensHotkeyHandler = NULL;
static const OSType localensHotkeySignature = 'LENS';

static OSStatus localensHandleHotkey(EventHandlerCallRef next, EventRef event, void *data) {
    EventHotKeyID key;
    OSStatus status = GetEventParameter(event, kEventParamDirectObject, typeEventHotKeyID,
        NULL, sizeof(key), NULL, &key);
    if (status != noErr || key.signature != localensHotkeySignature) return eventNotHandledErr;
    localensNativeHotkeyEvent(key.id, GetEventKind(event) == kEventHotKeyPressed);
    return noErr;
}

void localensRegisterHotkey(uint32_t id, uint32_t modifiers, uint32_t key) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (!localensNativeHotkeyAlive(id)) return;
        OSStatus status = noErr;
        EventHotKeyRef hotkey = NULL;
        EventTargetRef target = GetEventDispatcherTarget();
        if (localensHotkeyHandler == NULL) {
            EventTypeSpec events[] = {
                {kEventClassKeyboard, kEventHotKeyPressed},
                {kEventClassKeyboard, kEventHotKeyReleased}
            };
            status = InstallEventHandler(target, localensHandleHotkey, 2, events, NULL, &localensHotkeyHandler);
        }
        if (status == noErr) {
            EventHotKeyID identity = {localensHotkeySignature, id};
            status = RegisterEventHotKey(key, modifiers, identity, target, 0, &hotkey);
        }
        localensNativeHotkeyRegistered(id, (int)status, (uintptr_t)hotkey);
    });
}

int localensUnregisterHotkey(uintptr_t reference) {
    return (int)UnregisterEventHotKey((EventHotKeyRef)reference);
}
