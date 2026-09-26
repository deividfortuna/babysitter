package ghclient

import (
	"errors"
	"testing"

	"github.com/google/go-github/v91/github"
)

func TestPaginateCollectsEveryPage(t *testing.T) {
	t.Parallel()
	pages := [][]int{{1, 2}, {3, 4}, {5}}
	var asked []int
	got, resp, err := paginate(func(page int) ([]int, *github.Response, error) {
		asked = append(asked, page)
		i := page
		if page == 0 {
			i = 0
		}
		r := &github.Response{}
		if i+1 < len(pages) {
			r.NextPage = i + 1
		}
		return pages[i], r, nil
	})
	if err != nil {
		t.Fatalf("paginate() error = %v", err)
	}
	if len(got) != 5 || got[4] != 5 {
		t.Fatalf("paginate() = %v", got)
	}
	if resp == nil || resp.NextPage != 0 {
		t.Fatalf("paginate() response = %+v", resp)
	}
	if len(asked) != 3 {
		t.Fatalf("asked for pages %v", asked)
	}
}

func TestPaginateStopsWithoutAResponse(t *testing.T) {
	t.Parallel()
	got, resp, err := paginate(func(int) ([]int, *github.Response, error) {
		return []int{7}, nil, nil
	})
	if err != nil {
		t.Fatalf("paginate() error = %v", err)
	}
	if resp != nil {
		t.Fatalf("paginate() response = %+v, want nil", resp)
	}
	if len(got) != 1 || got[0] != 7 {
		t.Fatalf("paginate() = %v, want the one page it was given", got)
	}
}

func TestPaginateReturnsTheFailure(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	if _, _, err := paginate(func(int) ([]int, *github.Response, error) {
		return nil, nil, boom
	}); !errors.Is(err, boom) {
		t.Fatalf("paginate() error = %v, want %v", err, boom)
	}
}
