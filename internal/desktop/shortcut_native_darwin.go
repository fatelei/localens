//go:build darwin

package desktop

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework Carbon
#include <stdint.h>
void localensRegisterHotkey(uint32_t id, uint32_t modifiers, uint32_t key);
int localensUnregisterHotkey(uintptr_t reference);
*/
import "C"

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"golang.design/x/hotkey"
)

var nativeShortcutID atomic.Uint32
var nativeShortcuts sync.Map

type macShortcut struct {
	mu        sync.Mutex
	ctx       context.Context
	canceled  bool
	completed chan int
	spec      ShortcutSpec
	id        uint32
	reference C.uintptr_t
	down, up  chan hotkey.Event
}

func NativeShortcut(spec ShortcutSpec) ShortcutHandle {
	return &macShortcut{ctx: context.Background(), completed: make(chan int, 1), spec: spec, id: nativeShortcutID.Add(1), down: make(chan hotkey.Event, 1), up: make(chan hotkey.Event, 1)}
}
func (h *macShortcut) SetContext(ctx context.Context) { h.ctx = ctx }
func (h *macShortcut) Register() error {
	var modifiers hotkey.Modifier
	for _, modifier := range h.spec.modifiers {
		modifiers |= modifier
	}
	nativeShortcuts.Store(h.id, h)
	C.localensRegisterHotkey(C.uint32_t(h.id), C.uint32_t(modifiers), C.uint32_t(h.spec.key))
	select {
	case status := <-h.completed:
		if status != 0 {
			nativeShortcuts.Delete(h.id)
			return fmt.Errorf("macOS hotkey registration failed (%d)", status)
		}
		return nil
	case <-h.ctx.Done():
		h.mu.Lock()
		h.canceled = true
		reference := h.reference
		h.reference = 0
		h.mu.Unlock()
		if reference != 0 {
			_ = C.localensUnregisterHotkey(reference)
		}
		nativeShortcuts.Delete(h.id)
		return h.ctx.Err()
	}
}
func (h *macShortcut) Unregister() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.reference == 0 {
		return nil
	}
	status := C.localensUnregisterHotkey(h.reference)
	if status != 0 {
		return fmt.Errorf("macOS hotkey release failed (%d)", status)
	}
	nativeShortcuts.Delete(h.id)
	h.reference = 0
	return nil
}

//export localensNativeHotkeyAlive
func localensNativeHotkeyAlive(id C.uint32_t) C.int {
	_, ok := nativeShortcuts.Load(uint32(id))
	if ok {
		return 1
	}
	return 0
}

//export localensNativeHotkeyRegistered
func localensNativeHotkeyRegistered(id C.uint32_t, status C.int, reference C.uintptr_t) {
	value, ok := nativeShortcuts.Load(uint32(id))
	if !ok {
		if reference != 0 {
			_ = C.localensUnregisterHotkey(reference)
		}
		return
	}
	h := value.(*macShortcut)
	h.mu.Lock()
	if h.canceled {
		h.mu.Unlock()
		if reference != 0 {
			_ = C.localensUnregisterHotkey(reference)
		}
		return
	}
	h.reference = reference
	h.mu.Unlock()
	h.completed <- int(status)
}
func (h *macShortcut) Keydown() <-chan hotkey.Event { return h.down }
func (h *macShortcut) Keyup() <-chan hotkey.Event   { return h.up }

//export localensNativeHotkeyEvent
func localensNativeHotkeyEvent(id C.uint32_t, pressed C.int) {
	value, ok := nativeShortcuts.Load(uint32(id))
	if !ok {
		return
	}
	h := value.(*macShortcut)
	events := h.up
	if pressed != 0 {
		events = h.down
	}
	// Never block the AppKit dispatcher while Go brings the window forward.
	select {
	case events <- hotkey.Event{}:
	default:
	}
}
