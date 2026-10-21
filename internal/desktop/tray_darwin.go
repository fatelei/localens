//go:build darwin

package desktop

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
void localensInstallStatusItem(void);
void localensRemoveStatusItem(void);
void localensSetTrayLanguage(int english);
*/
import "C"
import "context"

var statusActions = make(chan int, 8)

//export localensStatusAction
func localensStatusAction(action C.int) {
	// Never block the AppKit main thread on a Go window operation.
	select {
	case statusActions <- int(action):
	default:
	}
}

// InstallTray submits native installation before subsequent language updates.
func InstallTray() {
	C.localensInstallStatusItem()
}

// RunTray serves an installed tray until the application lifetime ends.
func RunTray(ctx context.Context, actions Actions) {
	defer C.localensRemoveStatusItem()
	for {
		select {
		case <-ctx.Done():
			return
		case action := <-statusActions:
			switch action {
			case 0:
				actions.ShowLauncher()
			case 1:
				actions.ShowSettings()
			case 2:
				actions.QuitApp()
			}
		}
	}
}

func SetTrayLanguage(english bool) {
	var flag C.int
	if english {
		flag = 1
	}
	C.localensSetTrayLanguage(flag)
}
