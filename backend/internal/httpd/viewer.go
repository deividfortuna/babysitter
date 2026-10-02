package httpd

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/deividfortuna/babysitter/internal/redact"
)

const viewerTTL = 30 * time.Minute

type Viewer struct {
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl"`
}

type ViewerFunc func(ctx context.Context) (Viewer, error)

var errNoViewer = errors.New("the daemon cannot ask GitHub who the token belongs to")

type viewerCache struct {
	fn ViewerFunc

	mu         sync.Mutex
	value      Viewer
	at         time.Time
	generation int
}

func (c *viewerCache) get(ctx context.Context) (Viewer, error) {
	c.mu.Lock()
	fresh := !c.at.IsZero() && time.Since(c.at) < viewerTTL
	value, generation := c.value, c.generation
	c.mu.Unlock()
	if fresh {
		return value, nil
	}
	if c.fn == nil {
		return Viewer{}, errNoViewer
	}
	v, err := c.fn(ctx)
	if err != nil {
		return Viewer{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation == generation {
		c.value, c.at = v, time.Now()
	}
	return v, nil
}

func (c *viewerCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = time.Time{}
	c.generation++
}

func (a *api) handleViewer(w http.ResponseWriter, r *http.Request) {
	v, err := a.viewer.get(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "viewer_unavailable", redact.Text(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, v)
}
