---
type: llm
focus: last_message
weight: 1
---
The answer configures the repository with `babysitter repo config`. Pass if all hold:

- It sets the checkout with `--checkout ~/code/billing` (or the same path written out).
- It turns on `--auto-start-mine` and `--auto-watch-dependabot`.
- It keeps or sets `--dependabot-scope patch`.
- It says that an approval in the name of the user needs `--dependabot-approval green`, or asks the user before it sets it, and does not set `green` silently. A branch that needs no review merges with the default `never`.
- It says only pull requests opened from now on start.

Fail if the answer invents commands or flags that the skill does not name, or starts a watch with `watch start` in place of the configuration.
