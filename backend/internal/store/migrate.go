package store

import (
	"context"
	"database/sql"
	"fmt"
)

var migrations = []string{
	`
CREATE TABLE repos (
    id             INTEGER PRIMARY KEY,
    owner          TEXT NOT NULL,
    name           TEXT NOT NULL,
    added_at       TEXT NOT NULL,
    last_synced_at TEXT,
    last_error     TEXT NOT NULL DEFAULT '',
    UNIQUE (owner, name)
);

CREATE TABLE pull_requests (
    repo_id             INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    number              INTEGER NOT NULL,
    github_id           INTEGER NOT NULL,
    title               TEXT NOT NULL,
    author              TEXT NOT NULL,
    state               TEXT NOT NULL CHECK (state IN ('open', 'closed', 'merged')),
    draft               INTEGER NOT NULL DEFAULT 0,
    base_ref            TEXT NOT NULL,
    head_ref            TEXT NOT NULL,
    head_sha            TEXT NOT NULL,
    html_url            TEXT NOT NULL,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    merged_at           TEXT,
    closed_at           TEXT,
    mergeable_state     TEXT NOT NULL DEFAULT '',
    review_decision     TEXT NOT NULL DEFAULT 'none',
    approvals           INTEGER NOT NULL DEFAULT 0,
    changes_requested   INTEGER NOT NULL DEFAULT 0,
    requested_reviewers TEXT NOT NULL DEFAULT '[]',
    labels              TEXT NOT NULL DEFAULT '[]',
    additions           INTEGER NOT NULL DEFAULT 0,
    deletions           INTEGER NOT NULL DEFAULT 0,
    ci_status           TEXT NOT NULL DEFAULT 'none',
    synced_at           TEXT NOT NULL,
    PRIMARY KEY (repo_id, number)
);

CREATE INDEX pull_requests_state_idx ON pull_requests (state);
`,
	`
CREATE TABLE pr_watch (
    owner            TEXT NOT NULL,
    name             TEXT NOT NULL,
    number           INTEGER NOT NULL,
    started_at       TEXT NOT NULL,
    last_snapshot_at TEXT NOT NULL,
    last_head_sha    TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (owner, name, number)
);

CREATE TABLE pr_watch_seen (
    owner   TEXT NOT NULL,
    name    TEXT NOT NULL,
    number  INTEGER NOT NULL,
    kind    TEXT NOT NULL CHECK (kind IN ('issue_comment', 'review_comment', 'review')),
    item_id INTEGER NOT NULL,
    seen_at TEXT NOT NULL,
    PRIMARY KEY (owner, name, number, kind, item_id),
    FOREIGN KEY (owner, name, number) REFERENCES pr_watch(owner, name, number) ON DELETE CASCADE
);

CREATE TABLE pr_watch_retries (
    owner    TEXT NOT NULL,
    name     TEXT NOT NULL,
    number   INTEGER NOT NULL,
    head_sha TEXT NOT NULL,
    retries  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (owner, name, number, head_sha),
    FOREIGN KEY (owner, name, number) REFERENCES pr_watch(owner, name, number) ON DELETE CASCADE
);
`,
	`
CREATE TEMP TABLE pr_watch_seen_old AS SELECT * FROM pr_watch_seen;
CREATE TEMP TABLE pr_watch_retries_old AS SELECT * FROM pr_watch_retries;

CREATE TABLE pr_watch_new (
    owner            TEXT NOT NULL COLLATE NOCASE,
    name             TEXT NOT NULL COLLATE NOCASE,
    number           INTEGER NOT NULL,
    started_at       TEXT NOT NULL,
    last_snapshot_at TEXT NOT NULL,
    last_head_sha    TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (owner, name, number)
);
INSERT INTO pr_watch_new (owner, name, number, started_at, last_snapshot_at, last_head_sha)
SELECT owner, name, number, MIN(started_at), MAX(last_snapshot_at), MAX(last_head_sha)
FROM pr_watch GROUP BY lower(owner), lower(name), number;

DROP TABLE pr_watch_seen;
DROP TABLE pr_watch_retries;
DROP TABLE pr_watch;
ALTER TABLE pr_watch_new RENAME TO pr_watch;

CREATE TABLE pr_watch_seen (
    owner   TEXT NOT NULL COLLATE NOCASE,
    name    TEXT NOT NULL COLLATE NOCASE,
    number  INTEGER NOT NULL,
    kind    TEXT NOT NULL CHECK (kind IN ('issue_comment', 'review_comment', 'review')),
    item_id INTEGER NOT NULL,
    seen_at TEXT NOT NULL,
    PRIMARY KEY (owner, name, number, kind, item_id),
    FOREIGN KEY (owner, name, number) REFERENCES pr_watch(owner, name, number) ON DELETE CASCADE
);
INSERT OR IGNORE INTO pr_watch_seen (owner, name, number, kind, item_id, seen_at)
SELECT owner, name, number, kind, item_id, seen_at FROM pr_watch_seen_old;

CREATE TABLE pr_watch_retries (
    owner    TEXT NOT NULL COLLATE NOCASE,
    name     TEXT NOT NULL COLLATE NOCASE,
    number   INTEGER NOT NULL,
    head_sha TEXT NOT NULL,
    retries  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (owner, name, number, head_sha),
    FOREIGN KEY (owner, name, number) REFERENCES pr_watch(owner, name, number) ON DELETE CASCADE
);
INSERT INTO pr_watch_retries (owner, name, number, head_sha, retries)
SELECT owner, name, number, head_sha, MAX(retries)
FROM pr_watch_retries_old GROUP BY lower(owner), lower(name), number, head_sha;

DROP TABLE pr_watch_seen_old;
DROP TABLE pr_watch_retries_old;
`,
	`
CREATE TABLE watches (
    id                 INTEGER PRIMARY KEY,
    owner              TEXT NOT NULL COLLATE NOCASE,
    name               TEXT NOT NULL COLLATE NOCASE,
    number             INTEGER NOT NULL,
    url                TEXT NOT NULL DEFAULT '',
    title              TEXT NOT NULL DEFAULT '',
    author             TEXT NOT NULL DEFAULT '',
    bot_login          TEXT NOT NULL DEFAULT '',
    head_ref           TEXT NOT NULL DEFAULT '',
    base_ref           TEXT NOT NULL DEFAULT '',
    source_dir         TEXT NOT NULL DEFAULT '',
    worktree_dir       TEXT NOT NULL DEFAULT '',
    work_branch        TEXT NOT NULL DEFAULT '',
    git_user_name      TEXT NOT NULL DEFAULT '',
    git_user_email     TEXT NOT NULL DEFAULT '',
    status             TEXT NOT NULL CHECK (status IN ('active', 'stopped')),
    stop_reason        TEXT NOT NULL DEFAULT '',
    include_existing   INTEGER NOT NULL DEFAULT 0,
    started_at         TEXT NOT NULL,
    stopped_at         TEXT,
    last_poll_at       TEXT,
    last_heartbeat_at  TEXT,
    last_error         TEXT NOT NULL DEFAULT '',
    consecutive_errors INTEGER NOT NULL DEFAULT 0,
    head_sha           TEXT NOT NULL DEFAULT '',
    pr_state           TEXT NOT NULL DEFAULT '',
    mergeable_state    TEXT NOT NULL DEFAULT '',
    check_states       TEXT NOT NULL DEFAULT '{}',
    green_sha          TEXT NOT NULL DEFAULT '',
    summary            TEXT NOT NULL DEFAULT '{}'
);

CREATE UNIQUE INDEX watches_active_idx ON watches (owner, name, number) WHERE status = 'active';

CREATE TABLE watch_activity (
    id          INTEGER PRIMARY KEY,
    watch_id    INTEGER NOT NULL REFERENCES watches(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL CHECK (kind IN (
                    'comment', 'review_comment', 'review',
                    'check_failed', 'check_recovered', 'checks_green',
                    'commit', 'behind', 'conflict', 'merged', 'closed',
                    'heartbeat', 'watch_started', 'watch_stopped',
                    'proposal_ready', 'proposal_stale', 'pushed', 'replied', 'agent_failed')),
    ref         TEXT NOT NULL,
    at          TEXT NOT NULL,
    actor       TEXT NOT NULL DEFAULT '',
    summary     TEXT NOT NULL DEFAULT '',
    url         TEXT NOT NULL DEFAULT '',
    payload     TEXT NOT NULL DEFAULT '{}',
    reported    INTEGER NOT NULL DEFAULT 0,
    reported_at TEXT,
    UNIQUE (watch_id, kind, ref)
);

CREATE INDEX watch_activity_watch_idx ON watch_activity (watch_id, id);

CREATE TABLE proposals (
    id                   INTEGER PRIMARY KEY,
    watch_id             INTEGER NOT NULL REFERENCES watches(id) ON DELETE CASCADE,
    kind                 TEXT NOT NULL CHECK (kind IN ('fix', 'reply', 'rebase')),
    status               TEXT NOT NULL CHECK (status IN (
                             'preparing', 'pending', 'approved', 'landing', 'landed',
                             'skipped', 'stale', 'failed', 'superseded')),
    decision             TEXT NOT NULL DEFAULT '' CHECK (decision IN ('', 'fix', 'already_resolved', 'decline', 'question')),
    ci_class             TEXT NOT NULL DEFAULT '' CHECK (ci_class IN ('', 'pr_caused', 'flaky_infra', 'broken_on_base', 'unrelated')),
    title                TEXT NOT NULL DEFAULT '',
    summary              TEXT NOT NULL DEFAULT '',
    root_cause           TEXT NOT NULL DEFAULT '',
    patch                TEXT NOT NULL DEFAULT '',
    files                TEXT NOT NULL DEFAULT '[]',
    commit_message       TEXT NOT NULL DEFAULT '',
    replies              TEXT NOT NULL DEFAULT '[]',
    verify_command       TEXT NOT NULL DEFAULT '',
    verify_exit_code     INTEGER NOT NULL DEFAULT -1,
    verify_output        TEXT NOT NULL DEFAULT '',
    verify_ok            INTEGER NOT NULL DEFAULT 0,
    head_sha_at_proposal TEXT NOT NULL DEFAULT '',
    base_sha_at_proposal TEXT NOT NULL DEFAULT '',
    result_sha           TEXT NOT NULL DEFAULT '',
    landed_sha           TEXT NOT NULL DEFAULT '',
    landed_at            TEXT,
    revision             INTEGER NOT NULL DEFAULT 1,
    parent_id            INTEGER,
    author_note          TEXT NOT NULL DEFAULT '',
    agent_session        TEXT NOT NULL DEFAULT '',
    error                TEXT NOT NULL DEFAULT '',
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL,
    decided_at           TEXT
);

CREATE INDEX proposals_watch_idx ON proposals (watch_id, id);

CREATE TABLE proposal_activity (
    proposal_id INTEGER NOT NULL REFERENCES proposals(id) ON DELETE CASCADE,
    activity_id INTEGER NOT NULL REFERENCES watch_activity(id) ON DELETE CASCADE,
    PRIMARY KEY (proposal_id, activity_id)
);
`,
	`
ALTER TABLE watches ADD COLUMN include_own INTEGER NOT NULL DEFAULT 0;
`,
	`
	ALTER TABLE watches ADD COLUMN provider TEXT NOT NULL DEFAULT 'claude';
	`,
	`
	ALTER TABLE watches ADD COLUMN model TEXT NOT NULL DEFAULT '';
	`,
	`
	ALTER TABLE proposals ADD COLUMN command TEXT NOT NULL DEFAULT '';
	`,
	`
	ALTER TABLE proposals ADD COLUMN command_at TEXT;
	`,
	`
	ALTER TABLE proposals ADD COLUMN reviewers TEXT NOT NULL DEFAULT '[]';
	`,
	`
	SELECT 1;
	`,
	`
	DROP TABLE proposal_activity;
	DROP TABLE proposals;

	CREATE TABLE watch_activity_new (
	    id          INTEGER PRIMARY KEY,
	    watch_id    INTEGER NOT NULL REFERENCES watches(id) ON DELETE CASCADE,
	    kind        TEXT NOT NULL CHECK (kind IN (
	                    'comment', 'review_comment', 'review',
	                    'check_failed', 'check_recovered', 'checks_green',
	                    'commit', 'behind', 'conflict', 'merged', 'closed',
	                    'heartbeat', 'watch_started', 'watch_stopped',
	                    'session_started', 'session_exited', 'nudged', 'agent_failed')),
	    ref         TEXT NOT NULL,
	    at          TEXT NOT NULL,
	    actor       TEXT NOT NULL DEFAULT '',
	    summary     TEXT NOT NULL DEFAULT '',
	    url         TEXT NOT NULL DEFAULT '',
	    payload     TEXT NOT NULL DEFAULT '{}',
	    reported    INTEGER NOT NULL DEFAULT 0,
	    reported_at TEXT,
	    nudged_at   TEXT,
	    UNIQUE (watch_id, kind, ref)
	);

	INSERT INTO watch_activity_new (id, watch_id, kind, ref, at, actor, summary, url, payload, reported, reported_at)
	SELECT id, watch_id, kind, ref, at, actor, summary, url, payload, reported, reported_at FROM watch_activity
	WHERE kind NOT IN ('proposal_ready', 'proposal_stale', 'pushed', 'replied', 'rereviewed');

	DROP TABLE watch_activity;
	ALTER TABLE watch_activity_new RENAME TO watch_activity;
	CREATE INDEX watch_activity_watch_idx ON watch_activity (watch_id, id);

	ALTER TABLE watches ADD COLUMN agent_session TEXT NOT NULL DEFAULT '';
	`,
	`
	ALTER TABLE watches ADD COLUMN approvals_required INTEGER NOT NULL DEFAULT 1;
	ALTER TABLE watches ADD COLUMN merge_method TEXT NOT NULL DEFAULT '';
	ALTER TABLE watches ADD COLUMN ready_since TEXT;
	ALTER TABLE watches ADD COLUMN ready_blockers TEXT NOT NULL DEFAULT '[]';

	CREATE TABLE watch_activity_new (
	    id          INTEGER PRIMARY KEY,
	    watch_id    INTEGER NOT NULL REFERENCES watches(id) ON DELETE CASCADE,
	    kind        TEXT NOT NULL CHECK (kind IN (
	                    'comment', 'review_comment', 'review',
	                    'check_failed', 'check_recovered', 'checks_green',
	                    'commit', 'behind', 'conflict', 'merged', 'closed',
	                    'heartbeat', 'watch_started', 'watch_stopped',
	                    'session_started', 'session_exited', 'nudged', 'agent_failed',
	                    'merge_ready', 'merge_failed')),
	    ref         TEXT NOT NULL,
	    at          TEXT NOT NULL,
	    actor       TEXT NOT NULL DEFAULT '',
	    summary     TEXT NOT NULL DEFAULT '',
	    url         TEXT NOT NULL DEFAULT '',
	    payload     TEXT NOT NULL DEFAULT '{}',
	    reported    INTEGER NOT NULL DEFAULT 0,
	    reported_at TEXT,
	    nudged_at   TEXT,
	    UNIQUE (watch_id, kind, ref)
	);

	INSERT INTO watch_activity_new (id, watch_id, kind, ref, at, actor, summary, url, payload, reported, reported_at, nudged_at)
	SELECT id, watch_id, kind, ref, at, actor, summary, url, payload, reported, reported_at, nudged_at FROM watch_activity;

	DROP TABLE watch_activity;
	ALTER TABLE watch_activity_new RENAME TO watch_activity;
	CREATE INDEX watch_activity_watch_idx ON watch_activity (watch_id, id);
	`,
	`
	CREATE TABLE watch_activity_new (
	    id          INTEGER PRIMARY KEY,
	    watch_id    INTEGER NOT NULL REFERENCES watches(id) ON DELETE CASCADE,
	    kind        TEXT NOT NULL CHECK (kind IN (
	                    'comment', 'review_comment', 'review',
	                    'check_failed', 'check_recovered', 'checks_green',
	                    'commit', 'behind', 'conflict', 'merged', 'closed',
	                    'heartbeat', 'watch_started', 'watch_stopped',
	                    'session_started', 'session_exited', 'nudged', 'agent_failed',
	                    'merge_ready', 'merge_failed', 'replied')),
	    ref         TEXT NOT NULL,
	    at          TEXT NOT NULL,
	    actor       TEXT NOT NULL DEFAULT '',
	    summary     TEXT NOT NULL DEFAULT '',
	    url         TEXT NOT NULL DEFAULT '',
	    payload     TEXT NOT NULL DEFAULT '{}',
	    reported    INTEGER NOT NULL DEFAULT 0,
	    reported_at TEXT,
	    nudged_at   TEXT,
	    UNIQUE (watch_id, kind, ref)
	);

	INSERT INTO watch_activity_new (id, watch_id, kind, ref, at, actor, summary, url, payload, reported, reported_at, nudged_at)
	SELECT id, watch_id, kind, ref, at, actor, summary, url, payload, reported, reported_at, nudged_at FROM watch_activity;

	DROP TABLE watch_activity;
	ALTER TABLE watch_activity_new RENAME TO watch_activity;
	CREATE INDEX watch_activity_watch_idx ON watch_activity (watch_id, id);
	`,
	`
	CREATE TABLE watch_activity_new (
	    id          INTEGER PRIMARY KEY,
	    watch_id    INTEGER NOT NULL REFERENCES watches(id) ON DELETE CASCADE,
	    kind        TEXT NOT NULL CHECK (kind IN (
	                    'comment', 'review_comment', 'review',
	                    'check_failed', 'check_recovered', 'checks_green',
	                    'commit', 'behind', 'conflict', 'merged', 'closed',
	                    'heartbeat', 'watch_started', 'watch_stopped',
	                    'session_started', 'session_exited', 'nudged', 'agent_failed',
	                    'merge_ready', 'merge_failed', 'replied', 'review_requested')),
	    ref         TEXT NOT NULL,
	    at          TEXT NOT NULL,
	    actor       TEXT NOT NULL DEFAULT '',
	    summary     TEXT NOT NULL DEFAULT '',
	    url         TEXT NOT NULL DEFAULT '',
	    payload     TEXT NOT NULL DEFAULT '{}',
	    reported    INTEGER NOT NULL DEFAULT 0,
	    reported_at TEXT,
	    nudged_at   TEXT,
	    UNIQUE (watch_id, kind, ref)
	);

	INSERT INTO watch_activity_new (id, watch_id, kind, ref, at, actor, summary, url, payload, reported, reported_at, nudged_at)
	SELECT id, watch_id, kind, ref, at, actor, summary, url, payload, reported, reported_at, nudged_at FROM watch_activity;

	DROP TABLE watch_activity;
	ALTER TABLE watch_activity_new RENAME TO watch_activity;
	CREATE INDEX watch_activity_watch_idx ON watch_activity (watch_id, id);
	`,
	`
	CREATE TABLE settings (
	    id                 INTEGER PRIMARY KEY CHECK (id = 1),
	    poll_interval_ms   INTEGER NOT NULL,
	    watch_interval_ms  INTEGER NOT NULL,
	    approvals_required INTEGER,
	    merge_method       TEXT NOT NULL DEFAULT '' CHECK (merge_method IN ('', 'squash', 'merge', 'rebase')),
	    include_existing   INTEGER NOT NULL DEFAULT 0,
	    include_own        INTEGER NOT NULL DEFAULT 0,
	    keep_worktree      INTEGER NOT NULL DEFAULT 0
	);

	INSERT INTO settings (id, poll_interval_ms, watch_interval_ms) VALUES (1, 60000, 180000);
	`,
	`
	CREATE TABLE notifications (
	    id         INTEGER PRIMARY KEY,
	    watch_id   INTEGER REFERENCES watches(id) ON DELETE CASCADE,
	    kind       TEXT NOT NULL CHECK (kind IN ('agent', 'review', 'checks', 'watch', 'merge')),
	    repo       TEXT NOT NULL DEFAULT '',
	    number     INTEGER NOT NULL DEFAULT 0,
	    title      TEXT NOT NULL,
	    body       TEXT NOT NULL,
	    url        TEXT NOT NULL DEFAULT '',
	    created_at TEXT NOT NULL,
	    read_at    TEXT
	);

	CREATE INDEX notifications_unread_idx ON notifications (read_at, id);

	ALTER TABLE settings ADD COLUMN notifications_enabled INTEGER NOT NULL DEFAULT 1;
	ALTER TABLE settings ADD COLUMN notification_sound INTEGER NOT NULL DEFAULT 1;
	`,
	`
	ALTER TABLE settings ADD COLUMN muted_notification_kinds TEXT NOT NULL DEFAULT '';
	`,
	`
	ALTER TABLE notifications ADD COLUMN silent INTEGER NOT NULL DEFAULT 0;
	`,
	`
	CREATE TABLE proposals (
	    id          INTEGER PRIMARY KEY,
	    watch_id    INTEGER NOT NULL REFERENCES watches(id) ON DELETE CASCADE,
	    number      INTEGER NOT NULL,
	    status      TEXT NOT NULL CHECK (status IN ('open', 'pending', 'released', 'failed', 'rejected', 'declined', 'superseded')),
	    head_sha    TEXT NOT NULL,
	    base_sha    TEXT NOT NULL,
	    work_sha    TEXT NOT NULL DEFAULT '',
	    has_push    INTEGER NOT NULL DEFAULT 0,
	    opened_at   TEXT NOT NULL,
	    ended_at    TEXT,
	    released_at TEXT,
	    error       TEXT NOT NULL DEFAULT '',
	    UNIQUE (watch_id, number)
	);

	CREATE UNIQUE INDEX proposals_active_idx ON proposals (watch_id) WHERE status IN ('open', 'pending');

	CREATE TABLE proposal_replies (
	    id          INTEGER PRIMARY KEY,
	    proposal_id INTEGER NOT NULL REFERENCES proposals(id) ON DELETE CASCADE,
	    in_reply_to INTEGER NOT NULL DEFAULT 0,
	    body        TEXT NOT NULL,
	    recorded_at TEXT NOT NULL,
	    posted_kind TEXT NOT NULL DEFAULT '' CHECK (posted_kind IN ('', 'issue_comment', 'review_comment', 'review')),
	    posted_id   INTEGER NOT NULL DEFAULT 0,
	    posted_url  TEXT NOT NULL DEFAULT '',
	    posted_at   TEXT,
	    error       TEXT NOT NULL DEFAULT ''
	);

	CREATE INDEX proposal_replies_proposal_idx ON proposal_replies (proposal_id, id);
	`,
	`
	ALTER TABLE proposal_replies ADD COLUMN dropped_at TEXT;
	`,
	`
	ALTER TABLE settings ADD COLUMN approval_mode TEXT NOT NULL DEFAULT 'auto' CHECK (approval_mode IN ('auto', 'manual'));
	ALTER TABLE settings ADD COLUMN auto_approve_rebase INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE watches ADD COLUMN approval_mode TEXT NOT NULL DEFAULT 'auto' CHECK (approval_mode IN ('auto', 'manual'));
	ALTER TABLE watches ADD COLUMN auto_approve_rebase INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE proposals ADD COLUMN approved_at TEXT;
	ALTER TABLE proposals ADD COLUMN decided_at TEXT;
	ALTER TABLE proposals ADD COLUMN push_rejected INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE proposals ADD COLUMN rebased_from TEXT NOT NULL DEFAULT '';
	ALTER TABLE proposals ADD COLUMN reason TEXT NOT NULL DEFAULT '';
	ALTER TABLE proposal_replies ADD COLUMN edited_body TEXT NOT NULL DEFAULT '';
	ALTER TABLE proposal_replies ADD COLUMN dropped INTEGER NOT NULL DEFAULT 0;
	`,
	`
	CREATE TABLE watch_activity_new (
	    id          INTEGER PRIMARY KEY,
	    watch_id    INTEGER NOT NULL REFERENCES watches(id) ON DELETE CASCADE,
	    kind        TEXT NOT NULL CHECK (kind IN (
	                    'comment', 'review_comment', 'review',
	                    'check_failed', 'check_recovered', 'checks_green',
	                    'commit', 'behind', 'conflict', 'merged', 'closed',
	                    'heartbeat', 'watch_started', 'watch_stopped',
	                    'session_started', 'session_exited', 'nudged', 'agent_failed',
	                    'merge_ready', 'merge_failed', 'replied', 'review_requested', 'proposal')),
	    ref         TEXT NOT NULL,
	    at          TEXT NOT NULL,
	    actor       TEXT NOT NULL DEFAULT '',
	    summary     TEXT NOT NULL DEFAULT '',
	    url         TEXT NOT NULL DEFAULT '',
	    payload     TEXT NOT NULL DEFAULT '{}',
	    reported    INTEGER NOT NULL DEFAULT 0,
	    reported_at TEXT,
	    nudged_at   TEXT,
	    UNIQUE (watch_id, kind, ref)
	);

	INSERT INTO watch_activity_new (id, watch_id, kind, ref, at, actor, summary, url, payload, reported, reported_at, nudged_at)
	SELECT id, watch_id, kind, ref, at, actor, summary, url, payload, reported, reported_at, nudged_at FROM watch_activity;

	DROP TABLE watch_activity;
	ALTER TABLE watch_activity_new RENAME TO watch_activity;
	CREATE INDEX watch_activity_watch_idx ON watch_activity (watch_id, id);
	`,
	`
	ALTER TABLE proposals ADD COLUMN start_sha TEXT NOT NULL DEFAULT '';
	`,
	`
	ALTER TABLE proposal_replies ADD COLUMN in_reply_kind TEXT NOT NULL DEFAULT '';
	UPDATE proposal_replies SET in_reply_kind = 'review_comment' WHERE in_reply_to != 0;
	`,
	`
	CREATE TABLE github_responses (
	    key           TEXT PRIMARY KEY,
	    etag          TEXT NOT NULL DEFAULT '',
	    last_modified TEXT NOT NULL DEFAULT '',
	    status        INTEGER NOT NULL,
	    header        TEXT NOT NULL DEFAULT '{}',
	    body          BLOB NOT NULL,
	    size          INTEGER NOT NULL
	);
	`,
	`
	ALTER TABLE watches ADD COLUMN taken_over_at TEXT;
	ALTER TABLE watches ADD COLUMN taken_over_pid INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE watches ADD COLUMN handback_start TEXT NOT NULL DEFAULT '';

	CREATE TABLE watch_activity_new (
	    id          INTEGER PRIMARY KEY,
	    watch_id    INTEGER NOT NULL REFERENCES watches(id) ON DELETE CASCADE,
	    kind        TEXT NOT NULL CHECK (kind IN (
	                    'comment', 'review_comment', 'review',
	                    'check_failed', 'check_recovered', 'checks_green',
	                    'commit', 'behind', 'conflict', 'merged', 'closed',
	                    'heartbeat', 'watch_started', 'watch_stopped',
	                    'session_started', 'session_exited', 'nudged', 'agent_failed',
	                    'merge_ready', 'merge_failed', 'replied', 'review_requested', 'proposal',
	                    'taken_over', 'handed_back')),
	    ref         TEXT NOT NULL,
	    at          TEXT NOT NULL,
	    actor       TEXT NOT NULL DEFAULT '',
	    summary     TEXT NOT NULL DEFAULT '',
	    url         TEXT NOT NULL DEFAULT '',
	    payload     TEXT NOT NULL DEFAULT '{}',
	    reported    INTEGER NOT NULL DEFAULT 0,
	    reported_at TEXT,
	    nudged_at   TEXT,
	    UNIQUE (watch_id, kind, ref)
	);

	INSERT INTO watch_activity_new (id, watch_id, kind, ref, at, actor, summary, url, payload, reported, reported_at, nudged_at)
	SELECT id, watch_id, kind, ref, at, actor, summary, url, payload, reported, reported_at, nudged_at FROM watch_activity;

	DROP TABLE watch_activity;
	ALTER TABLE watch_activity_new RENAME TO watch_activity;
	CREATE INDEX watch_activity_watch_idx ON watch_activity (watch_id, id);
	`,
	`
	CREATE TABLE repo_config (
	    repo_id               INTEGER PRIMARY KEY REFERENCES repos(id) ON DELETE CASCADE,
	    checkout_dir          TEXT NOT NULL DEFAULT '',
	    own_since             TEXT,
	    include_drafts        INTEGER NOT NULL DEFAULT 0,
	    dependabot_since      TEXT,
	    provider              TEXT NOT NULL DEFAULT '',
	    model                 TEXT NOT NULL DEFAULT '',
	    approval_mode         TEXT NOT NULL DEFAULT '' CHECK (approval_mode IN ('', 'auto', 'manual')),
	    merge_method          TEXT NOT NULL DEFAULT '' CHECK (merge_method IN ('', 'squash', 'merge', 'rebase')),
	    approvals_set         INTEGER NOT NULL DEFAULT 0,
	    approvals_count       INTEGER,
	    include_existing      INTEGER,
	    dependabot_scope      TEXT NOT NULL DEFAULT 'patch' CHECK (dependabot_scope IN ('patch', 'minor', 'major')),
	    dependabot_approval   TEXT NOT NULL DEFAULT 'never' CHECK (dependabot_approval IN ('never', 'ask', 'green')),
	    dependabot_limit      INTEGER NOT NULL DEFAULT 1 CHECK (dependabot_limit >= 1)
	);

	CREATE TABLE auto_start_claims (
	    repo_id    INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
	    number     INTEGER NOT NULL,
	    claimed_at TEXT NOT NULL,
	    PRIMARY KEY (repo_id, number)
	);

	ALTER TABLE pull_requests ADD COLUMN assignees TEXT NOT NULL DEFAULT '[]';
	ALTER TABLE pull_requests ADD COLUMN fork INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE pull_requests ADD COLUMN update_type TEXT NOT NULL DEFAULT '';

	ALTER TABLE watches ADD COLUMN auto_reason TEXT NOT NULL DEFAULT '' CHECK (auto_reason IN ('', 'mine', 'assigned', 'dependabot'));
	ALTER TABLE watches ADD COLUMN merge_when_ready INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE watches ADD COLUMN update_type TEXT NOT NULL DEFAULT '';

	CREATE TABLE watch_activity_new (
	    id          INTEGER PRIMARY KEY,
	    watch_id    INTEGER NOT NULL REFERENCES watches(id) ON DELETE CASCADE,
	    kind        TEXT NOT NULL CHECK (kind IN (
	                    'comment', 'review_comment', 'review',
	                    'check_failed', 'check_recovered', 'checks_green',
	                    'commit', 'behind', 'conflict', 'merged', 'closed',
	                    'heartbeat', 'watch_started', 'watch_stopped',
	                    'session_started', 'session_exited', 'nudged', 'agent_failed',
	                    'merge_ready', 'merge_failed', 'replied', 'review_requested', 'proposal',
	                    'taken_over', 'handed_back', 'auto_started', 'approved', 'approval_asked')),
	    ref         TEXT NOT NULL,
	    at          TEXT NOT NULL,
	    actor       TEXT NOT NULL DEFAULT '',
	    summary     TEXT NOT NULL DEFAULT '',
	    url         TEXT NOT NULL DEFAULT '',
	    payload     TEXT NOT NULL DEFAULT '{}',
	    reported    INTEGER NOT NULL DEFAULT 0,
	    reported_at TEXT,
	    nudged_at   TEXT,
	    UNIQUE (watch_id, kind, ref)
	);

	INSERT INTO watch_activity_new (id, watch_id, kind, ref, at, actor, summary, url, payload, reported, reported_at, nudged_at)
	SELECT id, watch_id, kind, ref, at, actor, summary, url, payload, reported, reported_at, nudged_at FROM watch_activity;

	DROP TABLE watch_activity;
	ALTER TABLE watch_activity_new RENAME TO watch_activity;
	CREATE INDEX watch_activity_watch_idx ON watch_activity (watch_id, id);

	CREATE TABLE notifications_new (
	    id         INTEGER PRIMARY KEY,
	    watch_id   INTEGER REFERENCES watches(id) ON DELETE CASCADE,
	    kind       TEXT NOT NULL CHECK (kind IN ('agent', 'review', 'checks', 'watch', 'merge', 'auto')),
	    repo       TEXT NOT NULL DEFAULT '',
	    number     INTEGER NOT NULL DEFAULT 0,
	    title      TEXT NOT NULL,
	    body       TEXT NOT NULL,
	    url        TEXT NOT NULL DEFAULT '',
	    created_at TEXT NOT NULL,
	    read_at    TEXT,
	    silent     INTEGER NOT NULL DEFAULT 0,
	    action     TEXT NOT NULL DEFAULT '' CHECK (action IN ('', 'approve_merge'))
	);

	INSERT INTO notifications_new (id, watch_id, kind, repo, number, title, body, url, created_at, read_at, silent)
	SELECT id, watch_id, kind, repo, number, title, body, url, created_at, read_at, silent FROM notifications;

	DROP TABLE notifications;
	ALTER TABLE notifications_new RENAME TO notifications;
	CREATE INDEX notifications_unread_idx ON notifications (read_at, id);
	`,
	`
	ALTER TABLE settings ADD COLUMN provider TEXT NOT NULL DEFAULT 'claude' CHECK (provider IN ('claude', 'copilot'));
	ALTER TABLE settings ADD COLUMN model TEXT NOT NULL DEFAULT '';

	ALTER TABLE repo_config ADD COLUMN auto_approve_rebase INTEGER;
	ALTER TABLE repo_config ADD COLUMN include_own INTEGER;
	ALTER TABLE repo_config ADD COLUMN keep_worktree INTEGER;

	ALTER TABLE watches ADD COLUMN keep_worktree INTEGER NOT NULL DEFAULT 0;
	UPDATE watches SET keep_worktree = (SELECT keep_worktree FROM settings WHERE id = 1);
	`,
	`
	ALTER TABLE settings ADD COLUMN effort TEXT NOT NULL DEFAULT '';
	ALTER TABLE repo_config ADD COLUMN effort TEXT NOT NULL DEFAULT '';
	ALTER TABLE watches ADD COLUMN effort TEXT NOT NULL DEFAULT '';
	`,
}

const freshSeed = `UPDATE settings SET approval_mode = 'manual' WHERE id = 1;`

func migrate(ctx context.Context, db *sql.DB) error {
	return migrateChain(ctx, db, len(migrations), true)
}

func migrateChain(ctx context.Context, db *sql.DB, target int, seed bool) error {
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version > len(migrations) {
		return fmt.Errorf("database schema version %d is newer than this build supports (%d)", version, len(migrations))
	}
	fresh := seed && version == 0
	for i := version; i < target; i++ {
		script := migrations[i]
		if fresh && i+1 == target {
			script += freshSeed
		}
		if err := applyMigration(ctx, db, i+1, script); err != nil {
			return err
		}
	}
	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, version int, script string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", version, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, script); err != nil {
		return fmt.Errorf("apply migration %d: %w", version, err)
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		return fmt.Errorf("set schema version %d: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", version, err)
	}
	return nil
}
