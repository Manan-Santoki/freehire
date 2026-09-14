#!/bin/sh
set -eu

worker=${1:?worker name required}
shift
budget=30m
lock=$worker
wait_seconds=0
case "$worker" in
  deduplicate) worker=reindex; lock=deduplicate; budget=2h; export REINDEX_DEDUP_ONLY=1 ;;
  reindex) lock=search; budget=4h; wait_seconds=900 ;;
  search-drain) lock=search; budget=15m ;;
  reindex-companies|build-suggestions) lock=search; budget=1h ;;
  enrich|tg-extract)
    if [ -z "${LLM_BASE_URL:-}" ] || [ -z "${LLM_API_KEY:-}" ] || [ -z "${LLM_MODEL:-}" ]; then
      echo "$worker: skipped because LLM configuration is incomplete"
      exit 0
    fi
    budget=15m
    ;;
  notify|remind|nudge)
    # Saved-search alerts, application reminders and nudges need a delivery channel.
    if [ -z "${NOTIFY_EMAIL_FROM:-}" ] && [ -z "${TELEGRAM_BOT_TOKEN:-}" ]; then
      echo "$worker: skipped because neither NOTIFY_EMAIL_FROM nor TELEGRAM_BOT_TOKEN is set"
      exit 0
    fi
    budget=10m
    ;;
  onboarding)
    if [ -z "${NOTIFY_EMAIL_FROM:-}" ] || [ -z "${ONBOARDING_REPLY_TO:-}" ]; then
      echo "$worker: skipped because NOTIFY_EMAIL_FROM or ONBOARDING_REPLY_TO is unset"
      exit 0
    fi
    ;;
  tg-ingest|liveness|rollup-views|recount-companies|rollup-stats|rollup-facets|rollup-company|capture-apply-form|auth-cleanup|similar-backfill) ;;
  *) echo "unsupported scheduled worker: $worker" >&2; exit 2 ;;
esac

mkdir -p /tmp/freehire-worker-locks
exec 9>"/tmp/freehire-worker-locks/$lock"
if ! flock -w "$wait_seconds" 9; then
  echo "$worker: deferred because $lock is busy"
  exit 0
fi
exec timeout --signal=TERM --kill-after=30s "$budget" "/app/$worker" "$@"
