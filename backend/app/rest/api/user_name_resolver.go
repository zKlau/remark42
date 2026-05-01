package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"
)

type UserNameResolver interface {
	Resolve(ctx context.Context, userID string) (name string, found bool, err error)
}

type userNameCacheEntry struct {
	name      string
	found     bool
	expiresAt time.Time
}

type SQLUserNameResolver struct {
	db      *sql.DB
	query   string
	ttl     time.Duration
	timeout time.Duration

	mu    sync.RWMutex
	cache map[string]userNameCacheEntry
}

func NewSQLUserNameResolver(db *sql.DB, query string, ttl, timeout time.Duration) (*SQLUserNameResolver, error) {
	if db == nil {
		return nil, errors.New("db can't be nil")
	}
	if query == "" {
		return nil, errors.New("query can't be empty")
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("ttl should be positive, got %s", ttl)
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("timeout should be positive, got %s", timeout)
	}

	return &SQLUserNameResolver{
		db:      db,
		query:   query,
		ttl:     ttl,
		timeout: timeout,
		cache:   make(map[string]userNameCacheEntry),
	}, nil
}

func (r *SQLUserNameResolver) Resolve(ctx context.Context, userID string) (string, bool, error) {
	if userID == "" {
		return "", false, nil
	}

	if name, found, ok := r.fromCache(userID); ok {
		return name, found, nil
	}

	qctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	var name string
	err := r.db.QueryRowContext(qctx, r.query, userID).Scan(&name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			r.putCache(userID, "", false)
			return "", false, nil
		}
		return "", false, err
	}

	if name == "" {
		r.putCache(userID, "", false)
		return "", false, nil
	}

	r.putCache(userID, name, true)
	return name, true, nil
}

func (r *SQLUserNameResolver) fromCache(userID string) (name string, found bool, ok bool) {
	now := time.Now()

	r.mu.RLock()
	entry, exists := r.cache[userID]
	r.mu.RUnlock()

	if !exists {
		return "", false, false
	}
	if now.After(entry.expiresAt) {
		r.mu.Lock()
		if current, foundCurrent := r.cache[userID]; foundCurrent && now.After(current.expiresAt) {
			delete(r.cache, userID)
		}
		r.mu.Unlock()
		return "", false, false
	}

	return entry.name, entry.found, true
}

func (r *SQLUserNameResolver) putCache(userID, name string, found bool) {
	r.mu.Lock()
	r.cache[userID] = userNameCacheEntry{name: name, found: found, expiresAt: time.Now().Add(r.ttl)}
	r.mu.Unlock()
}