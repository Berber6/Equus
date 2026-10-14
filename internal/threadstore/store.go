// Package threadstore persists conversation sessions as append-only JSONL
// files, so a chat can be resumed later.
package threadstore

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Godric-W/Equus/internal/llm"
)

// Meta describes a session.
type Meta struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Model     string    `json:"model"`
	CWD       string    `json:"cwd"`
	Title     string    `json:"title"`
}

// Store reads and writes session files under a directory.
type Store struct {
	dir string
	mu  sync.Mutex
}

// New builds a store rooted at dir (e.g. ~/.equus/sessions).
func New(dir string) *Store {
	return &Store{dir: dir}
}

// Dir reports the store's directory.
func (s *Store) Dir() string { return s.dir }

// SetTitle rewrites the last meta line with a new title (appending a fresh
// meta record, which wins on load).
func (s *Store) SetTitle(id, title string) error {
	meta, err := s.readMeta(id)
	if err != nil {
		return err
	}
	meta.Title = strings.TrimSpace(title)
	file, err := os.OpenFile(s.path(id), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := json.Marshal(record{Type: recordMeta, Meta: &meta})
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = file.Write(data)
	return err
}

// Delete removes a session file.
func (s *Store) Delete(id string) error {
	return os.Remove(s.path(id))
}

// recordType tags each JSONL line.
type recordType string

const (
	recordMeta recordType = "meta"
	recordItem recordType = "item"
)

type record struct {
	Type recordType        `json:"type"`
	Meta *Meta             `json:"meta,omitempty"`
	Item *llm.ResponseItem `json:"item,omitempty"`
}

// Writer appends records for one session.
type Writer struct {
	store *Store
	id    string
	file  *os.File
	mu    sync.Mutex
}

// Create starts a new session file with the given metadata. The metadata ID is
// filled in when empty.
func (s *Store) Create(meta Meta) (*Writer, error) {
	if strings.TrimSpace(meta.ID) == "" {
		meta.ID = newSessionID()
	}
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = time.Now()
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return nil, err
	}
	path := s.path(meta.ID)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	writer := &Writer{store: s, id: meta.ID, file: file}
	if err := writer.write(record{Type: recordMeta, Meta: &meta}); err != nil {
		file.Close()
		return nil, err
	}
	return writer, nil
}

// ID reports the session id.
func (w *Writer) ID() string { return w.id }

// Append records one conversation item.
func (w *Writer) Append(item llm.ResponseItem) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.write(record{Type: recordItem, Item: &item})
}

// Close releases the underlying file.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *Writer) write(r record) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.file.Write(data)
	return err
}

// Load reads a session's metadata and items. Corrupt trailing lines are
// skipped rather than failing the whole load.
func (s *Store) Load(id string) (Meta, []llm.ResponseItem, error) {
	file, err := os.Open(s.path(id))
	if err != nil {
		return Meta{}, nil, err
	}
	defer file.Close()

	var meta Meta
	var items []llm.ResponseItem
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var r record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			continue // skip a corrupt line
		}
		switch r.Type {
		case recordMeta:
			if r.Meta != nil {
				meta = *r.Meta
			}
		case recordItem:
			if r.Item != nil {
				items = append(items, *r.Item)
			}
		}
	}
	if meta.ID == "" {
		meta.ID = id
	}
	return meta, items, nil
}

// List returns session metadata, newest first.
func (s *Store) List() ([]Meta, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var metas []Meta
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".jsonl")
		meta, err := s.readMeta(id)
		if err != nil {
			continue
		}
		metas = append(metas, meta)
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].CreatedAt.After(metas[j].CreatedAt) })
	return metas, nil
}

// Latest returns the most recent session's metadata.
func (s *Store) Latest() (Meta, error) {
	metas, err := s.List()
	if err != nil {
		return Meta{}, err
	}
	if len(metas) == 0 {
		return Meta{}, fmt.Errorf("no saved sessions")
	}
	return metas[0], nil
}

// readMeta reads only the first meta line of a session file.
func (s *Store) readMeta(id string) (Meta, error) {
	file, err := os.Open(s.path(id))
	if err != nil {
		return Meta{}, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var r record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			continue
		}
		if r.Type == recordMeta && r.Meta != nil {
			meta := *r.Meta
			if meta.ID == "" {
				meta.ID = id
			}
			return meta, nil
		}
	}
	return Meta{ID: id}, nil
}

func (s *Store) path(id string) string {
	return filepath.Join(s.dir, id+".jsonl")
}

// newSessionID builds a sortable, readable session id.
func newSessionID() string {
	now := time.Now()
	suffix := now.UnixNano() % 100000
	return fmt.Sprintf("%s-%05d", now.Format("20060102-150405"), suffix)
}

// Recorder adapts a Writer to the agent.Recorder interface.
type Recorder struct{ Writer *Writer }

// Record appends one item, ignoring write errors (persistence is best-effort).
func (r Recorder) Record(item llm.ResponseItem) {
	if r.Writer != nil {
		_ = r.Writer.Append(item)
	}
}
