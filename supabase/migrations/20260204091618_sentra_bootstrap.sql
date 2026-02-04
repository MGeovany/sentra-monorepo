
-- Sentra bootstrap schema for Supabase Postgres.

-- Vault key (E2EE wrapper)
create table if not exists public.vault_keys (
  user_id uuid primary key references auth.users(id) on delete cascade,
  doc jsonb not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

-- Idempotency keys (for /push)
create table if not exists public.idempotency_keys (
  user_id uuid not null references auth.users(id) on delete cascade,
  scope text not null,
  idem_key text not null,
  status text not null check (status in ('in_progress', 'done')),
  response_json jsonb,
  expires_at timestamptz not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint idempotency_keys_pkey primary key (user_id, scope, idem_key)
);

create index if not exists idx_idempotency_keys_expires_at
  on public.idempotency_keys (expires_at);

-- Machines: remove the “name must be unique” constraint; keep (user_id, machine_id) unique.
alter table public.machines
  drop constraint if exists uniq_user_machine_name;

drop index if exists public.uniq_user_machine_name;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'uniq_user_machine_id'
      and conrelid = 'public.machines'::regclass
  ) then
    alter table public.machines
      add constraint uniq_user_machine_id unique (user_id, machine_id);
  end if;
end $$;
