CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

 CREATE TABLE users (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  email       TEXT NOT NULL UNIQUE,
  api_token   TEXT NOT NULL UNIQUE,
  created_at  TIMESTAMP WITH TIME ZONE DEFAULT now()
);

CREATE TABLE projects (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  root_path   TEXT NOT NULL,
  created_at  TIMESTAMP WITH TIME ZONE DEFAULT now()
);
CREATE TABLE machines (
  id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  machine_name  TEXT NOT NULL,
  created_at    TIMESTAMP WITH TIME ZONE DEFAULT now()
);
CREATE TABLE commits (
  id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  project_id   UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  machine_id   UUID NOT NULL REFERENCES machines(id),
  message      TEXT NOT NULL,
  created_at   TIMESTAMP WITH TIME ZONE DEFAULT now()
);
CREATE TABLE commit_files (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  commit_id   UUID NOT NULL REFERENCES commits(id) ON DELETE CASCADE,
  file_path   TEXT NOT NULL,
  file_hash   TEXT NOT NULL,
  created_at  TIMESTAMP WITH TIME ZONE DEFAULT now()
);
CREATE TABLE file_blobs (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  commit_file_id  UUID NOT NULL REFERENCES commit_files(id) ON DELETE CASCADE,
  blob            TEXT NOT NULL,
  created_at      TIMESTAMP WITH TIME ZONE DEFAULT now()
);


CREATE INDEX idx_projects_user_id ON projects(user_id);
CREATE INDEX idx_machines_user_id ON machines(user_id);
CREATE INDEX idx_commits_user_id ON commits(user_id);
CREATE INDEX idx_commits_project_id ON commits(project_id);
CREATE INDEX idx_commit_files_commit_id ON commit_files(commit_id);

CREATE UNIQUE INDEX uniq_user_project_root
ON projects(user_id, root_path);

CREATE UNIQUE INDEX uniq_user_machine
ON machines(user_id, machine_name);

