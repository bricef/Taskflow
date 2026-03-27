-- Actors
CREATE TABLE actors (
    name TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('human', 'ai_agent')),
    role TEXT NOT NULL CHECK (role IN ('admin', 'member', 'read_only')),
    api_key_hash TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    active INTEGER NOT NULL DEFAULT 1
);

-- Boards
CREATE TABLE boards (
    slug TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    workflow TEXT NOT NULL,
    next_task_num INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    deleted INTEGER NOT NULL DEFAULT 0
);

-- Tasks
CREATE TABLE tasks (
    board_slug TEXT NOT NULL REFERENCES boards(slug),
    num INTEGER NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL,
    priority TEXT NOT NULL DEFAULT 'none' CHECK (priority IN ('critical','high','medium','low','none')),
    tags TEXT NOT NULL DEFAULT '[]',
    assignee TEXT REFERENCES actors(name),
    due_date TEXT,
    created_by TEXT NOT NULL REFERENCES actors(name),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    deleted INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (board_slug, num)
);

CREATE INDEX idx_tasks_board_state ON tasks(board_slug, state) WHERE deleted = 0;
CREATE INDEX idx_tasks_assignee ON tasks(assignee) WHERE deleted = 0;

-- Comments
CREATE TABLE comments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    board_slug TEXT NOT NULL,
    task_num INTEGER NOT NULL,
    actor TEXT NOT NULL REFERENCES actors(name),
    body TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT,
    FOREIGN KEY (board_slug, task_num) REFERENCES tasks(board_slug, num)
);

CREATE INDEX idx_comments_task ON comments(board_slug, task_num);

-- Dependencies
CREATE TABLE dependencies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_board TEXT NOT NULL,
    task_num INTEGER NOT NULL,
    depends_on_board TEXT NOT NULL,
    depends_on_num INTEGER NOT NULL,
    dep_type TEXT NOT NULL CHECK (dep_type IN ('depends_on', 'relates_to')),
    created_by TEXT NOT NULL REFERENCES actors(name),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    FOREIGN KEY (task_board, task_num) REFERENCES tasks(board_slug, num),
    FOREIGN KEY (depends_on_board, depends_on_num) REFERENCES tasks(board_slug, num),
    UNIQUE (task_board, task_num, depends_on_board, depends_on_num, dep_type)
);

CREATE INDEX idx_dependencies_task ON dependencies(task_board, task_num);
CREATE INDEX idx_dependencies_dep ON dependencies(depends_on_board, depends_on_num);

-- Attachments
CREATE TABLE attachments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    board_slug TEXT NOT NULL,
    task_num INTEGER NOT NULL,
    ref_type TEXT NOT NULL CHECK (ref_type IN ('url', 'file', 'git_commit', 'git_branch', 'git_pr')),
    reference TEXT NOT NULL,
    label TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES actors(name),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    FOREIGN KEY (board_slug, task_num) REFERENCES tasks(board_slug, num)
);

CREATE INDEX idx_attachments_task ON attachments(board_slug, task_num);

-- Audit log (append-only)
CREATE TABLE audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    board_slug TEXT NOT NULL,
    task_num INTEGER,
    actor TEXT NOT NULL,
    action TEXT NOT NULL,
    detail TEXT NOT NULL DEFAULT '{}',
    timestamp TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_audit_log_board ON audit_log(board_slug);
CREATE INDEX idx_audit_log_task ON audit_log(board_slug, task_num);

-- Webhooks
CREATE TABLE webhooks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL,
    events TEXT NOT NULL DEFAULT '[]',
    board_slug TEXT REFERENCES boards(slug),
    secret TEXT NOT NULL,
    active INTEGER NOT NULL DEFAULT 1,
    created_by TEXT NOT NULL REFERENCES actors(name),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
