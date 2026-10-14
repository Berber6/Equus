package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Godric-W/Equus/internal/llm"
	"github.com/Godric-W/Equus/internal/threadstore"
)

func TestPickerNavigationAndSelect(t *testing.T) {
	metas := []threadstore.Meta{{ID: "a"}, {ID: "b"}}
	loaded := ""
	model, err := NewModel(context.Background(), Options{
		Session:  newTestSession(t, "hi"),
		Sessions: metas,
		LoadSession: func(id string) ([]llm.ResponseItem, error) {
			loaded = id
			return []llm.ResponseItem{llm.UserMessage("prev")}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if model.picker == nil {
		t.Fatal("picker not opened")
	}
	// Move down and select.
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	if model.picker.selected != 1 {
		t.Fatalf("selected = %d", model.picker.selected)
	}
	updated, command, _ := model.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.picker != nil {
		t.Fatal("picker should close after select")
	}
	// The load runs as a tea.Cmd; execute it to observe the effect.
	if command == nil {
		t.Fatal("expected a load command")
	}
	if msg := command(); msg != nil {
		// apply the resulting message
		if loadedMsg, ok := msg.(sessionLoadedMsg); ok {
			model.applyLoadedSession(loadedMsg)
		}
	}
	if loaded != "b" {
		t.Fatalf("loaded = %q, want b", loaded)
	}
}

func TestPickerEscapeCloses(t *testing.T) {
	model, _ := NewModel(context.Background(), Options{Session: newTestSession(t, "hi"), Sessions: []threadstore.Meta{{ID: "a"}}})
	updated, _, _ := model.handlePickerKey(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	if model.picker != nil {
		t.Fatal("esc should close the picker")
	}
}

func TestApplyLoadedSessionRestoresHistory(t *testing.T) {
	model, _ := NewModel(context.Background(), Options{Session: newTestSession(t, "hi")})
	model.resize(80, 24)
	model.applyLoadedSession(sessionLoadedMsg{id: "x", items: []llm.ResponseItem{llm.UserMessage("previous question")}})
	if len(model.session.History()) != 1 {
		t.Fatalf("history = %d", len(model.session.History()))
	}
	if model.sessionID != "x" {
		t.Fatalf("sessionID = %q", model.sessionID)
	}
	if !contains(model.transcript, "previous question") {
		t.Fatalf("transcript missing restored line: %v", model.transcript)
	}
}

func TestDeleteConfirmFlow(t *testing.T) {
	deleted := ""
	model, _ := NewModel(context.Background(), Options{
		Session:   newTestSession(t, "hi"),
		SessionID: "s1",
		DeleteFn:  func(id string) error { deleted = id; return nil },
	})
	model.resize(80, 24)
	model.confirmDeleteSession()
	if !model.deleteConfirm {
		t.Fatal("delete not staged")
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	model = updated.(Model)
	if model.deleteConfirm {
		t.Fatal("confirm not resolved")
	}
	if deleted != "s1" {
		t.Fatalf("deleted = %q", deleted)
	}
}

func contains(lines []string, needle string) bool {
	for _, line := range lines {
		if strings.Contains(line, needle) {
			return true
		}
	}
	return false
}
