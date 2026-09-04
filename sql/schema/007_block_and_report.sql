-- +goose Up
CREATE TABLE blocks(
    blocker_id UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    blocked_id UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    PRIMARY KEY (blocker_id, blocked_id),
    CONSTRAINT chk_no_self_block CHECK (blocker_id <> blocked_id)
);

CREATE INDEX idx_blocks_blocked_id ON blocks(blocked_id);

CREATE TABLE reports(
    report_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reporter_id UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    reported_id UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    reason TEXT NOT NULL,
    details TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    CONSTRAINT chk_no_self_report CHECK (reporter_id <> reported_id)
);

CREATE INDEX idx_reports_reported_id ON reports(reported_id);

-- +goose Down
DROP TABLE reports;
DROP TABLE blocks;
