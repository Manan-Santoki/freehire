-- Add "sources to avoid" and "companies to avoid" sets to the single user profile,
-- alongside the existing "skills to avoid" set (0039). Empty by default: a user need
-- not exclude anything. Stored as trimmed, lowercased, deduplicated text arrays by
-- internal/identity/userprofile — excluded_sources holds jobs.source values (the crawl
-- adapter/board, e.g. greenhouse/workday/djinni), excluded_companies holds company_slug
-- values. Neither has a corresponding "wanted" set, unlike skills/excluded_skills, so
-- there is no cross-set exclusivity rule for these two.
ALTER TABLE public.user_profiles
    ADD COLUMN excluded_sources text[] NOT NULL DEFAULT '{}'::text[];

ALTER TABLE public.user_profiles
    ADD COLUMN excluded_companies text[] NOT NULL DEFAULT '{}'::text[];
