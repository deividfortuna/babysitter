package cli

import "testing"

func TestShellOf(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		goos string
		env  map[string]string
		want string
	}{
		{name: "SHELL wins on unix", goos: "darwin", env: map[string]string{"SHELL": "/bin/zsh"}, want: "/bin/zsh"},
		{name: "unix without SHELL", goos: "linux", want: "/bin/sh"},
		{name: "SHELL wins on windows", goos: "windows", env: map[string]string{"SHELL": "bash", "COMSPEC": `C:\Windows\system32\cmd.exe`}, want: "bash"},
		{name: "windows without SHELL takes COMSPEC", goos: "windows", env: map[string]string{"COMSPEC": `C:\Windows\system32\cmd.exe`}, want: `C:\Windows\system32\cmd.exe`},
		{name: "windows without SHELL or COMSPEC", goos: "windows", want: "cmd.exe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			getenv := func(key string) string { return tt.env[key] }
			if got := shellOf(tt.goos, getenv); got != tt.want {
				t.Errorf("shellOf(%q) = %q, want %q", tt.goos, got, tt.want)
			}
		})
	}
}
