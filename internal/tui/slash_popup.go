package tui

// slashPopupMaxVisible caps how many matches the popup shows at once.
const slashPopupMaxVisible = 6

// slashPopup is the command-completion overlay shown above the composer.
type slashPopup struct {
	items     []slashCommand
	selected  int
	dismissed string
}

// sync recomputes the matches for the current input.
func (p *slashPopup) sync(input string) {
	if p.dismissed != "" && p.dismissed == input {
		p.items = nil
		p.selected = 0
		return
	}
	if p.dismissed != "" && p.dismissed != input {
		p.dismissed = ""
	}
	p.items = filterSlashCommands(input)
	if p.selected >= len(p.items) {
		p.selected = 0
	}
}

// active reports whether the popup should be shown.
func (p *slashPopup) active() bool { return len(p.items) > 0 }

// move shifts the selection, wrapping around.
func (p *slashPopup) move(delta int) {
	if len(p.items) == 0 {
		return
	}
	p.selected = (p.selected + delta + len(p.items)) % len(p.items)
}

// selected returns the highlighted command.
func (p *slashPopup) selectedCommand() (slashCommand, bool) {
	if p.selected < 0 || p.selected >= len(p.items) {
		return "", false
	}
	return p.items[p.selected], true
}

// dismiss hides the popup until the input changes.
func (p *slashPopup) dismiss(input string) {
	p.dismissed = input
	p.items = nil
	p.selected = 0
}

// popupView renders the completion list above the composer (empty when hidden).
func (m Model) popupView() []string {
	if !m.popup.active() {
		return nil
	}
	visible := m.popup.items
	if len(visible) > slashPopupMaxVisible {
		visible = visible[:slashPopupMaxVisible]
	}
	rows := make([]string, 0, len(visible))
	for index, c := range visible {
		marker := "  "
		style := m.palette.dim()
		if index == m.popup.selected {
			marker = "› "
			style = m.palette.accent()
		}
		rows = append(rows, style.Render(marker+"/"+c.Name()+"  "+c.Description()))
	}
	return rows
}
