package app

import (
	"localens/frontend"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestFrontendBindingContractSurvivesPackageMoves(t *testing.T) {
	script, err := frontend.Assets.ReadFile("app.js")
	if err != nil {
		t.Fatal(err)
	}
	typ := reflect.TypeFor[*App]()
	namespace := "window.go?." + strings.ReplaceAll(typ.Elem().String(), ".", "?.")
	if !strings.Contains(string(script), namespace) {
		t.Fatalf("frontend does not target Wails binding %s", namespace)
	}
	calls := regexp.MustCompile(`call\('([A-Za-z]+)'`).FindAllSubmatch(script, -1)
	if len(calls) == 0 {
		t.Fatal("no backend calls found")
	}
	for _, call := range calls {
		name := string(call[1])
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("frontend calls missing backend method %s", name)
		}
	}
}
