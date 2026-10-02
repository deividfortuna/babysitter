package httpd

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

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
	fn     ViewerFunc
	flight singleflight.Group

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
	v, err, _ := c.flight.Do(strconv.Itoa(generation), func() (any, error) {
		return c.fn(context.WithoutCancel(ctx))
	})
	if err != nil {
		return Viewer{}, err
	}
	viewer := v.(Viewer)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation == generation {
		c.value, c.at = viewer, time.Now()
	}
	return viewer, nil
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
