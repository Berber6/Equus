package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func names(cmds []slashCommand) string {
	out := make([]string, len(cmds))
	for i, c := range cmds {
		out[i] = c.Name()
	}
	return strings.Join(out, ",")
}

func TestFilterSlashCommands(t *testing.T) {
	if got := filterSlashCommands("hello"); got != nil {
		t.Fatalf("plain text should not filter: %v", names(got))
	}
	if got := filterSlashCommands("/"); len(got) != len(slashCommands()) {
		t.Fatalf("/ should list all, got %d", len(got))
	}
	got := names(filterSlashCommands("/co"))
	if !strings.Contains(got, "compact") || strings.Contains(got, "help") {
		t.Fatalf("/co matches = %s", got)
	}
	if cl := names(filterSlashCommands("/cl")); !strings.Contains(cl, "clear") {
		t.Fatalf("/cl matches = %s", cl)
	}
	// A space disables filtering.
	if got := filterSlashCommands("/help "); got != nil {
		t.Fatalf("space should disable popup: %v", names(got))
	}
	if got := filterSlashCommands("/zzz"); len(got) != 0 {
		t.Fatalf("no matches expected: %v", names(got))
	}
}

func TestPopupMoveAndSelect(t *testing.T) {
	p := slashPopup{}
	p.sync("/")
	if !p.active() {
		t.Fatal("popup should be active")
	}
	first := p.selected
	p.move(1)
	if p.selected == first {
		t.Fatal("move should change selection")
	}
	// Moving the full cycle returns to the start.
	for i := 0; i < len(p.items); i++ {
		p.move(1)
	}
	if p.selected != first+1 {
		t.Fatalf("cycle wrap failed: %d", p.selected)
	}
}

func TestPopupEnterCompletes(t *testing.T) {
	model, _ := NewModel(context.Background(), Options{Session: newTestSession(t, "hi")})
	model.resize(80, 24)
	model.input.SetValue("/co")
	model.popup.sync(model.input.Value())
	if !model.popup.active() {
		t.Fatal("popup not active")
	}
	// Enter while a bare prefix completes instead of submitting.
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.running {
		t.Fatal("enter on a prefix must not start a turn")
	}
	if got := model.input.Value(); !strings.HasPrefix(got, "/") || !strings.HasSuffix(got, " ") {
		t.Fatalf("completion = %q", got)
	}
}

func TestPopupTabCompletes(t *testing.T) {
	model, _ := NewModel(context.Background(), Options{Session: newTestSession(t, "hi")})
	model.resize(80, 24)
	model.input.SetValue("/hel")
	model.popup.sync(model.input.Value())
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(Model)
	if model.input.Value() != "/help " {
		t.Fatalf("tab completion = %q", model.input.Value())
	}
}

func TestPopupEscDismisses(t *testing.T) {
	model, _ := NewModel(context.Background(), Options{Session: newTestSession(t, "hi")})
	model.resize(80, 24)
	model.input.SetValue("/")
	model.popup.sync(model.input.Value())
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	if model.popup.active() {
		t.Fatal("esc should dismiss the popup")
	}
}
