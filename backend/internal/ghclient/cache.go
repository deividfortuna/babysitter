package ghclient

import (
	"bytes"
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync"
)

const maxCacheEntries = 4096

const maxCacheBytes = 4 << 20

const maxCacheTotalBytes = 64 << 20

var (
	sharedMu    sync.Mutex
	sharedCache = newResponseCache(maxCacheEntries, maxCacheTotalBytes)
)

type CachedResponse struct {
	ETag         string
	LastModified string
	Status       int
	Header       http.Header
	Body         []byte
}

type ResponseStore interface {
	CachedResponse(ctx context.Context, key string) (CachedResponse, bool, error)
	PutCachedResponse(ctx context.Context, key string, r CachedResponse) error
	DeleteCachedResponse(ctx context.Context, key string) error
}

func KeepResponses(rs ResponseStore) {
	c := newResponseCache(maxCacheEntries, maxCacheTotalBytes)
	c.disk = rs
	sharedMu.Lock()
	defer sharedMu.Unlock()
	sharedCache = c
}

func currentCache() *responseCache {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	return sharedCache
}

type entry struct {
	etag     string
	modified string
	header   http.Header
	body     []byte
	status   int
}

func entryOf(r CachedResponse) entry {
	return entry{etag: r.ETag, modified: r.LastModified, header: r.Header, body: r.Body, status: r.Status}
}

func (e entry) response() CachedResponse {
	return CachedResponse{ETag: e.etag, LastModified: e.modified, Header: e.header, Body: e.body, Status: e.status}
}

type responseCache struct {
	mu         sync.Mutex
	maxEntries int
	maxBytes   int
	bytes      int
	order      *list.List
	byKey      map[string]*list.Element
	disk       ResponseStore
}

type node struct {
	key string
	e   entry
}

func newResponseCache(maxEntries, maxBytes int) *responseCache {
	return &responseCache{
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
		order:      list.New(),
		byKey:      map[string]*list.Element{},
	}
}

func (c *responseCache) get(key string) (entry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.byKey[key]
	if !ok {
		return entry{}, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*node).e, true
}

func (c *responseCache) put(key string, e entry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.byKey[key]; ok {
		held := el.Value.(*node)
		c.bytes += len(e.body) - len(held.e.body)
		held.e = e
		c.order.MoveToFront(el)
	} else {
		c.byKey[key] = c.order.PushFront(&node{key: key, e: e})
		c.bytes += len(e.body)
	}
	for c.order.Len() > 1 && (c.order.Len() > c.maxEntries || c.bytes > c.maxBytes) {
		c.drop(c.order.Back())
	}
}

func (c *responseCache) remove(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.byKey[key]; ok {
		c.drop(el)
	}
}

func (c *responseCache) lookup(ctx context.Context, key string) (entry, bool) {
	if e, ok := c.get(key); ok {
		return e, true
	}
	if c.disk == nil {
		return entry{}, false
	}
	r, ok, err := c.disk.CachedResponse(ctx, key)
	if err != nil || !ok {
		return entry{}, false
	}
	e := entryOf(r)
	c.put(key, e)
	return e, true
}

func (c *responseCache) keep(ctx context.Context, key string, e entry) {
	c.put(key, e)
	if c.disk != nil {
		_ = c.disk.PutCachedResponse(ctx, key, e.response())
	}
}

func (c *responseCache) forget(ctx context.Context, key string) {
	c.remove(key)
	if c.disk != nil {
		_ = c.disk.DeleteCachedResponse(ctx, key)
	}
}

func (c *responseCache) drop(el *list.Element) {
	held := el.Value.(*node)
	c.order.Remove(el)
	delete(c.byKey, held.key)
	c.bytes -= len(held.e.body)
}

type cachingTransport struct {
	base  http.RoundTripper
	cache *responseCache
}

func (t *cachingTransport) transport() http.RoundTripper {
	if t.base != nil {
		return t.base
	}
	return http.DefaultTransport
}

func (t *cachingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		return t.transport().RoundTrip(req)
	}
	ctx := req.Context()
	key := cacheKey(req)
	cached, hit := t.cache.lookup(ctx, key)
	if hit {
		req = withValidators(req, cached)
	}
	resp, err := t.transport().RoundTrip(req)
	if err != nil {
		return resp, err
	}
	if hit && resp.StatusCode == http.StatusNotModified {
		return replay(cached, resp), nil
	}
	return t.store(ctx, key, resp)
}

func withValidators(req *http.Request, cached entry) *http.Request {
	out := req.Clone(req.Context())
	if cached.etag != "" && out.Header.Get("If-None-Match") == "" {
		out.Header.Set("If-None-Match", cached.etag)
	}
	if cached.modified != "" && out.Header.Get("If-Modified-Since") == "" {
		out.Header.Set("If-Modified-Since", cached.modified)
	}
	return out
}

func replay(cached entry, resp *http.Response) *http.Response {
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	for key, values := range cached.header {
		if _, ok := resp.Header[key]; !ok {
			resp.Header[key] = slices.Clone(values)
		}
	}
	resp.StatusCode = cached.status
	resp.Status = fmt.Sprintf("%d %s", cached.status, http.StatusText(cached.status))
	resp.Body = io.NopCloser(bytes.NewReader(cached.body))
	resp.ContentLength = int64(len(cached.body))
	return resp
}

func (t *cachingTransport) store(ctx context.Context, key string, resp *http.Response) (*http.Response, error) {
	etag, modified := resp.Header.Get("ETag"), resp.Header.Get("Last-Modified")
	if resp.StatusCode != http.StatusOK || (etag == "" && modified == "") {
		t.cache.forget(ctx, key)
		return resp, nil
	}
	body, whole, err := readAtMost(resp.Body, maxCacheBytes)
	if err != nil {
		resp.Body.Close()
		return nil, err
	}
	if !whole {
		t.cache.forget(ctx, key)
		resp.Body = readCloser{Reader: io.MultiReader(bytes.NewReader(body), resp.Body), Closer: resp.Body}
		return resp, nil
	}
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	t.cache.keep(ctx, key, entry{
		etag:     etag,
		modified: modified,
		header:   resp.Header.Clone(),
		body:     body,
		status:   resp.StatusCode,
	})
	return resp, nil
}

type readCloser struct {
	io.Reader
	io.Closer
}

func readAtMost(r io.Reader, limit int) (b []byte, whole bool, err error) {
	b, err = io.ReadAll(io.LimitReader(r, int64(limit)+1))
	if err != nil {
		return nil, false, err
	}
	return b, len(b) <= limit, nil
}

func cacheKey(req *http.Request) string {
	sum := sha256.New()
	for _, part := range []string{
		req.Method,
		req.URL.String(),
		req.Header.Get("Accept"),
		req.Header.Get("X-GitHub-Api-Version"),
		req.Header.Get("Authorization"),
	} {
		fmt.Fprintf(sum, "%d:%s", len(part), part)
	}
	return hex.EncodeToString(sum.Sum(nil))
}
