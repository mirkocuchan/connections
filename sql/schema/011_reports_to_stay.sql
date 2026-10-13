-- +goose Up
-- el reporte tiene que sobrevivir si se borran las cuentas o el chat
ALTER TABLE reports ALTER COLUMN reporter_id DROP NOT NULL;
ALTER TABLE reports ALTER COLUMN reported_id DROP NOT NULL;

ALTER TABLE reports DROP CONSTRAINT reports_reporter_id_fkey;
ALTER TABLE reports DROP CONSTRAINT reports_reported_id_fkey;
ALTER TABLE reports ADD CONSTRAINT reports_reporter_id_fkey FOREIGN KEY (reporter_id) REFERENCES users(user_id) ON DELETE SET NULL;
ALTER TABLE reports ADD CONSTRAINT reports_reported_id_fkey FOREIGN KEY (reported_id) REFERENCES users(user_id) ON DELETE SET NULL;

ALTER TABLE reports ADD COLUMN chat_id UUID REFERENCES chats(chat_id) ON DELETE SET NULL;
ALTER TABLE reports ADD COLUMN reported_username TEXT;
ALTER TABLE reports ADD COLUMN status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'reviewed', 'actioned', 'dismissed'));
ALTER TABLE reports ADD COLUMN reviewed_at TIMESTAMP;

CREATE INDEX idx_reports_status ON reports(status);

CREATE TABLE report_messages(
    report_message_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    report_id UUID NOT NULL REFERENCES reports(report_id) ON DELETE CASCADE,
    author_role TEXT NOT NULL CHECK (author_role IN ('reporter', 'reported')),
    content TEXT NOT NULL,
    sent_at TIMESTAMP NOT NULL
);

CREATE INDEX idx_report_messages_report_id ON report_messages(report_id);

-- +goose Down
DROP TABLE report_messages;
DROP INDEX idx_reports_status;
ALTER TABLE reports DROP COLUMN reviewed_at;
ALTER TABLE reports DROP COLUMN status;
ALTER TABLE reports DROP COLUMN reported_username;
ALTER TABLE reports DROP COLUMN chat_id;
ALTER TABLE reports DROP CONSTRAINT reports_reporter_id_fkey;
ALTER TABLE reports DROP CONSTRAINT reports_reported_id_fkey;
ALTER TABLE reports ADD CONSTRAINT reports_reporter_id_fkey FOREIGN KEY (reporter_id) REFERENCES users(user_id) ON DELETE CASCADE;
ALTER TABLE reports ADD CONSTRAINT reports_reported_id_fkey FOREIGN KEY (reported_id) REFERENCES users(user_id) ON DELETE CASCADE;
-- el NOT NULL no se restaura a propósito: puede haber reportes con cuentas ya borradas