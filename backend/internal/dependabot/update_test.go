package dependabot

import "testing"

const groupedBody = "Bumps the aws group with 2 updates: [aws-sdk](https://github.com/aws/aws-sdk) and [aws-cdk](https://github.com/aws/aws-cdk).\n\n" +
	"Updates `aws-sdk` from 1.2.3 to 1.2.9\n" +
	"<details>\n<summary>Release notes</summary>\n<p>Moves the client from v1 to v3 of the protocol.</p>\n</details>\n\n" +
	"Updates `aws-cdk` from 2.10.0 to 3.0.1\n" +
	"<details>\n<summary>Commits</summary>\n</details>\n"

func TestUpdateType(t *testing.T) {
	tests := []struct {
		name  string
		title string
		body  string
		want  Level
	}{
		{name: "patch", title: "Bump lodash from 4.17.20 to 4.17.21", want: Patch},
		{name: "minor with a prefix", title: "build(deps): bump golang.org/x/net from 0.33.0 to 0.34.0", want: Minor},
		{name: "major", title: "Bump react from 18.3.1 to 19.0.0 in /frontend", want: Major},
		{name: "v prefix", title: "Bump actions/checkout from v4.1.0 to v4.2.0", want: Minor},
		{name: "requirement", title: "Update rake requirement from ~> 13.0 to ~> 13.1", want: Minor},
		{name: "grouped takes the highest", title: "Bump the aws group with 2 updates", body: groupedBody, want: Major},
		{name: "release notes do not count", title: "Bump x from 1.0.0 to 1.0.1", body: "<details>\nfrom 1.0.0 to 9.0.0\n</details>", want: Patch},
		{name: "a sha is unknown", title: "Bump lib from `a1b2c3d` to `e4f5a6b`", want: Major},
		{name: "nothing to read", title: "Update the lockfile", want: Major},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := UpdateType(tc.title, tc.body); got != tc.want {
				t.Fatalf("UpdateType(%q) = %s, want %s", tc.title, got, tc.want)
			}
		})
	}
}

func TestWithin(t *testing.T) {
	tests := []struct {
		level, scope Level
		want         bool
	}{
		{Patch, Patch, true},
		{Minor, Patch, false},
		{Minor, Minor, true},
		{Major, Minor, false},
		{Major, Major, true},
		{"", Major, false},
	}
	for _, tc := range tests {
		if got := Within(tc.level, tc.scope); got != tc.want {
			t.Errorf("Within(%s, %s) = %v, want %v", tc.level, tc.scope, got, tc.want)
		}
	}
}
