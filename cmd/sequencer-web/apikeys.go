package main

import (
	"cmp"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

type apiKey struct {
	Name      string
	Value     string
	CreatedAt time.Time
}

// keyStore holds API keys in memory only; they are lost on restart.
type keyStore struct {
	mu   sync.RWMutex
	keys map[string]apiKey
}

func newKeyStore() *keyStore {
	return &keyStore{keys: map[string]apiKey{}}
}

func (s *keyStore) add(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("name of key is required")
	}

	b := make([]byte, 16)
	_, _ = rand.Read(b) // never fails since Go 1.24
	k := apiKey{Name: name, Value: "sk_" + hex.EncodeToString(b), CreatedAt: time.Now().UTC()}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.keys[name]; ok {
		return fmt.Errorf("a key named %q already exists", name)
	}
	s.keys[name] = k
	return nil
}

func (s *keyStore) remove(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.keys, name)
}

// list returns all keys, oldest first.
func (s *keyStore) list() []apiKey {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]apiKey, 0, len(s.keys))
	for _, k := range s.keys {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b apiKey) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), strings.Compare(a.Name, b.Name))
	})
	return keys
}
