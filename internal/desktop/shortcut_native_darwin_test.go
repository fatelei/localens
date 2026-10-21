//go:build darwin

package desktop

import (
	"context"
	"testing"
	"time"
)

func TestNativeShortcutCancellationWithoutAppKitLoop(t *testing.T) {
	spec, err := ParseShortcut("Ctrl+Q")
	if err != nil {
		t.Fatal(err)
	}
	handle := NativeShortcut(spec).(*macShortcut)
	ctx, cancel := context.WithCancel(t.Context())
	handle.SetContext(ctx)
	cancel()
	done := make(chan error, 1)
	go func() { done <- handle.Register() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled registration succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("registration blocked waiting for AppKit")
	}
	if _, exists := nativeShortcuts.Load(handle.id); exists {
		t.Fatal("canceled registration still published")
	}
}
func TestNativeShortcutEventIsNonblockingAndIgnoresInactiveIDs(t *testing.T) {
	spec, _ := ParseShortcut("Ctrl+Q")
	handle := NativeShortcut(spec).(*macShortcut)
	nativeShortcuts.Store(uint32(123456), handle)
	defer nativeShortcuts.Delete(uint32(123456))
	// Constants are passed through the actual C-exported bridge.
	for range 3 {
		localensNativeHotkeyEvent(123456, 1)
	}
	if len(handle.down) != 1 {
		t.Fatal("press event not delivered")
	}
	<-handle.down
	nativeShortcuts.Delete(uint32(123456))
	localensNativeHotkeyEvent(123456, 1)
	if len(handle.down) != 0 {
		t.Fatal("inactive shortcut delivered")
	}
}
