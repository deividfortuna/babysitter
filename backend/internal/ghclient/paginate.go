package ghclient

import "github.com/google/go-github/v91/github"

func paginate[T any](fetch func(page int) ([]T, *github.Response, error)) ([]T, *github.Response, error) {
	var all []T
	for page := 0; ; {
		items, resp, err := fetch(page)
		if err != nil {
			return nil, resp, err
		}
		all = append(all, items...)
		if resp == nil || resp.NextPage == 0 {
			return all, resp, nil
		}
		page = resp.NextPage
	}
}
