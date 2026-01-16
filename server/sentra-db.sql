-- Sentra database schema (idempotent / migration-style).
--
-- Notes:
-- - This file is safe to run multiple times.
-- - It prefers additive changes (CREATE IF NOT EXISTS / ALTER TABLE ADD COLUMN IF NOT EXISTS).
-- - In Supabase, the canonical user table is `auth.users`.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Projects
CREATE TABLE IF NOT EXISTS projects (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id     UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  root_path   TEXT NOT NULL,
  created_at  TIMESTAMP WITH TIME ZONE DEFAULT now()
);

-- Machines
CREATE TABLE IF NOT EXISTS machines (
  id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id       UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  machine_id    TEXT NOT NULL,
  machine_name  TEXT NOT NULL,
  created_at    TIMESTAMP WITH TIME ZONE DEFAULT now()
);

-- If the table existed before, ensure new columns are present.
ALTER TABLE machines ADD COLUMN IF NOT EXISTS machine_id TEXT;
ALTER TABLE machines ADD COLUMN IF NOT EXISTS machine_name TEXT;

-- Commits
CREATE TABLE IF NOT EXISTS commits (
  id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id      UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  project_id   UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  machine_id   UUID NOT NULL REFERENCES machines(id),
  message      TEXT NOT NULL,
  created_at   TIMESTAMP WITH TIME ZONE DEFAULT now()
);

-- Commit files
CREATE TABLE IF NOT EXISTS commit_files (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  commit_id   UUID NOT NULL REFERENCES commits(id) ON DELETE CASCADE,
  file_path   TEXT NOT NULL,
  file_hash   TEXT NOT NULL,
  created_at  TIMESTAMP WITH TIME ZONE DEFAULT now()
);

-- File blobs
CREATE TABLE IF NOT EXISTS file_blobs (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  commit_file_id  UUID NOT NULL REFERENCES commit_files(id) ON DELETE CASCADE,
  blob            TEXT NOT NULL,
  created_at      TIMESTAMP WITH TIME ZONE DEFAULT now()
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_projects_user_id ON projects(user_id);
CREATE INDEX IF NOT EXISTS idx_machines_user_id ON machines(user_id);
CREATE INDEX IF NOT EXISTS idx_commits_user_id ON commits(user_id);
CREATE INDEX IF NOT EXISTS idx_commits_project_id ON commits(project_id);
CREATE INDEX IF NOT EXISTS idx_commit_files_commit_id ON commit_files(commit_id);

-- Uniques
CREATE UNIQUE INDEX IF NOT EXISTS uniq_user_project_root
ON projects(user_id, root_path);

CREATE UNIQUE INDEX IF NOT EXISTS uniq_user_machine_name
ON machines(user_id, machine_name);

CREATE UNIQUE INDEX IF NOT EXISTS uniq_user_machine_id
ON machines(user_id, machine_id);

-- Foreign key fixes (migration-style)
--
-- If tables were created in an earlier iteration referencing a custom `public.users`
-- table, switch them to Supabase's `auth.users`.

DO $$
BEGIN
  -- projects.user_id
  IF EXISTS (
    SELECT 1
    FROM pg_constraint c
    WHERE c.conname = 'projects_user_id_fkey'
      AND c.conrelid = 'public.projects'::regclass
      AND pg_get_constraintdef(c.oid) LIKE '%REFERENCES users%'
  ) THEN
    EXECUTE 'ALTER TABLE public.projects DROP CONSTRAINT projects_user_id_fkey';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint c
    WHERE c.conname = 'projects_user_id_fkey'
      AND c.conrelid = 'public.projects'::regclass
  ) THEN
    EXECUTE 'ALTER TABLE public.projects ADD CONSTRAINT projects_user_id_fkey FOREIGN KEY (user_id) REFERENCES auth.users(id) ON DELETE CASCADE';
  END IF;

  -- machines.user_id
  IF EXISTS (
    SELECT 1
    FROM pg_constraint c
    WHERE c.conname = 'machines_user_id_fkey'
      AND c.conrelid = 'public.machines'::regclass
      AND pg_get_constraintdef(c.oid) LIKE '%REFERENCES users%'
  ) THEN
    EXECUTE 'ALTER TABLE public.machines DROP CONSTRAINT machines_user_id_fkey';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint c
    WHERE c.conname = 'machines_user_id_fkey'
      AND c.conrelid = 'public.machines'::regclass
  ) THEN
    EXECUTE 'ALTER TABLE public.machines ADD CONSTRAINT machines_user_id_fkey FOREIGN KEY (user_id) REFERENCES auth.users(id) ON DELETE CASCADE';
  END IF;

  -- commits.user_id
  IF EXISTS (
    SELECT 1
    FROM pg_constraint c
    WHERE c.conname = 'commits_user_id_fkey'
      AND c.conrelid = 'public.commits'::regclass
      AND pg_get_constraintdef(c.oid) LIKE '%REFERENCES users%'
  ) THEN
    EXECUTE 'ALTER TABLE public.commits DROP CONSTRAINT commits_user_id_fkey';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint c
    WHERE c.conname = 'commits_user_id_fkey'
      AND c.conrelid = 'public.commits'::regclass
  ) THEN
    EXECUTE 'ALTER TABLE public.commits ADD CONSTRAINT commits_user_id_fkey FOREIGN KEY (user_id) REFERENCES auth.users(id) ON DELETE CASCADE';
  END IF;
END $$;
