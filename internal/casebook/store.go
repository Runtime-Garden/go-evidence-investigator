// Package casebook implements a small, durable, integrity-checked review ledger.
// Each case is a 0600 JSON file, replaced atomically and containing a hash-chained
// sequence of review events. It intentionally avoids a database dependency.
package casebook

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

var idPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
var ErrNotFound = errors.New("case not found")
var ErrVersionConflict = errors.New("case revision conflict")

type ReviewEvent struct {
	Version      int       `json:"version"`
	Status       string    `json:"status"`
	Actor        string    `json:"actor"`
	Note         string    `json:"note"`
	At           time.Time `json:"at"`
	PreviousHash string    `json:"previous_hash"`
	Hash         string    `json:"hash"`
}
type Case struct {
	ID        string          `json:"id"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	Version   int             `json:"version"`
	Status    string          `json:"status"`
	Decision  string          `json:"decision"`
	Report    json.RawMessage `json:"report"`
	Events    []ReviewEvent   `json:"events"`
}
type Store struct {
	Dir string
	mu  sync.Mutex
}

func New(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("CASE_DIR is empty")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &Store{Dir: dir}, nil
}
func (s *Store) file(id string) (string, error) {
	if !idPattern.MatchString(id) {
		return "", errors.New("invalid case ID")
	}
	return filepath.Join(s.Dir, id+".json"), nil
}
func id() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func eventHash(e ReviewEvent) string {
	v := struct {
		Version      int       `json:"version"`
		Status       string    `json:"status"`
		Actor        string    `json:"actor"`
		Note         string    `json:"note"`
		At           time.Time `json:"at"`
		PreviousHash string    `json:"previous_hash"`
	}{e.Version, e.Status, e.Actor, e.Note, e.At, e.PreviousHash}
	b, _ := json.Marshal(v)
	d := sha256.Sum256(b)
	return hex.EncodeToString(d[:])
}
func check(c Case) error {
	if !idPattern.MatchString(c.ID) || len(c.Events) == 0 || c.Version != len(c.Events) || c.Status != c.Events[len(c.Events)-1].Status {
		return errors.New("corrupt case metadata")
	}
	previous := ""
	for i, e := range c.Events {
		if e.Version != i+1 || e.PreviousHash != previous || e.Hash != eventHash(e) {
			return errors.New("case review chain integrity failure")
		}
		previous = e.Hash
	}
	return nil
}
func (s *Store) save(c Case) error {
	path, err := s.file(c.ID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(s.Dir, ".case-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	if d, e := os.Open(s.Dir); e == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
func (s *Store) load(id string) (Case, error) {
	path, err := s.file(id)
	if err != nil {
		return Case{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Case{}, ErrNotFound
	}
	if err != nil {
		return Case{}, err
	}
	var c Case
	if err = json.Unmarshal(data, &c); err != nil {
		return Case{}, err
	}
	if err = check(c); err != nil {
		return Case{}, err
	}
	return c, nil
}
func (s *Store) Create(report any, decision string) (Case, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	body, err := json.Marshal(report)
	if err != nil {
		return Case{}, err
	}
	ident, err := id()
	if err != nil {
		return Case{}, err
	}
	now := time.Now().UTC()
	e := ReviewEvent{Version: 1, Status: "pending_review", Actor: "system", Note: "Investigation created for human review", At: now}
	e.Hash = eventHash(e)
	c := Case{ID: ident, CreatedAt: now, UpdatedAt: now, Version: 1, Status: e.Status, Decision: decision, Report: body, Events: []ReviewEvent{e}}
	if err = s.save(c); err != nil {
		return Case{}, err
	}
	return c, nil
}
func (s *Store) Get(id string) (Case, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.load(id) }
func (s *Store) Review(id string, version int, status, actor, note string) (Case, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.load(id)
	if err != nil {
		return Case{}, err
	}
	if c.Version != version {
		return Case{}, ErrVersionConflict
	}
	if c.Status != "pending_review" {
		return Case{}, errors.New("case already reviewed")
	}
	if status != "approved" && status != "rejected" && status != "needs_more_evidence" {
		return Case{}, errors.New("unsupported review status")
	}
	if actor == "" || len(actor) > 100 || len(note) > 2000 {
		return Case{}, errors.New("invalid reviewer or note")
	}
	if status != "approved" && len(note) < 3 {
		return Case{}, errors.New("review note required")
	}
	now := time.Now().UTC()
	e := ReviewEvent{Version: version + 1, Status: status, Actor: actor, Note: note, At: now, PreviousHash: c.Events[len(c.Events)-1].Hash}
	e.Hash = eventHash(e)
	c.Version++
	c.Status = status
	c.UpdatedAt = now
	c.Events = append(c.Events, e)
	if err = s.save(c); err != nil {
		return Case{}, fmt.Errorf("persist review: %w", err)
	}
	return c, nil
}
func (s *Store) List(limit int) ([]Case, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit < 1 || limit > 100 {
		limit = 30
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, err
	}
	out := make([]Case, 0, limit)
	// IDs do not imply time order; List is for review queues, sorted newest-first.
	for _, entry := range entries {
		if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		ident := entry.Name()[:len(entry.Name())-5]
		c, err := s.load(ident)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].CreatedAt.After(out[i].CreatedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
