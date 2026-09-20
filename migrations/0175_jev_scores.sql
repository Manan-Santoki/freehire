-- Per-(user, job) Jev fit score + its work queue. The score is the seniority/role/
-- blocker-aware replacement for the deterministic jobmatch coverage badge and the
-- source of the personalized "For You" feed ordering. Only feed-eligible pairs are ever
-- scored (see internal/candidate/jeveligible); the feed reads only stored rows, so the
-- feed IS the eligible set by construction.
--
-- Staleness is quintuple-stamped like user_job_analysis: model (the RESOLVED Jev model,
-- so an upgrade auto-invalidates), score_version (JEVSCORE_VERSION bump), the profile
-- fingerprint (specializations+seniorities+skills+location), cv_uploaded_at, and
-- job_content_hash. A recompute overwrites the row and all stamps. FKs cascade.
CREATE TABLE public.user_job_scores (
    user_id             bigint      NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    job_id              bigint      NOT NULL REFERENCES public.jobs(id)  ON DELETE CASCADE,
    match_pct           smallint    NOT NULL,
    match_raw           real        NOT NULL,
    match_confidence    real        NOT NULL,
    role_category       text        NOT NULL,
    role_confidence     real        NOT NULL,
    has_required_stack  real        NOT NULL,
    fits_level          real        NOT NULL,
    hard_blocker        real        NOT NULL,
    verdict             text        NOT NULL,
    model               text        NOT NULL,
    score_version       integer     NOT NULL,
    profile_fingerprint text        NOT NULL,
    cv_uploaded_at      timestamptz,
    job_content_hash    text,
    scored_at           timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, job_id)
);

-- Feed ordering (per user, best first) and verdict filtering.
CREATE INDEX user_job_scores_feed_idx ON public.user_job_scores (user_id, match_pct DESC);
CREATE INDEX user_job_scores_verdict_idx ON public.user_job_scores (user_id, verdict);

-- Reference-only work queue, mirroring semantic_outbox: one live entry per (user, job),
-- freshest job first, lease + dead-letter. Rows carry no copy of the job.
CREATE TABLE public.job_score_outbox (
    id             bigint      NOT NULL,
    user_id        bigint      NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    job_id         bigint      NOT NULL REFERENCES public.jobs(id)  ON DELETE CASCADE,
    target_version integer     NOT NULL,
    job_posted_at  timestamptz,
    attempts       integer     NOT NULL DEFAULT 0,
    claimed_at     timestamptz,
    failed_at      timestamptz,
    last_error     text        NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE public.job_score_outbox
    ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (SEQUENCE NAME public.job_score_outbox_id_seq);

ALTER TABLE ONLY public.job_score_outbox
    ADD CONSTRAINT job_score_outbox_pkey PRIMARY KEY (id);

-- The enqueue dedup key: one live entry per (user, job).
ALTER TABLE ONLY public.job_score_outbox
    ADD CONSTRAINT job_score_outbox_user_job_key UNIQUE (user_id, job_id);

-- Claim order: freshest job first over claimable (not dead-lettered) rows.
CREATE INDEX job_score_outbox_claim_idx
    ON public.job_score_outbox (job_posted_at DESC NULLS LAST, job_id DESC)
    WHERE (failed_at IS NULL);
