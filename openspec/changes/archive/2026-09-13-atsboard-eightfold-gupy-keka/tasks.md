## 1. Recognition

- [x] 1.1 Add `eightfold.ai` → `eightfold`, `gupy.io` → `gupy`, `keka.com` → `keka` as
      `subdomain`-mode entries in `atsBoards`
- [x] 1.2 Add one `TestRecognize` case per platform (vacancy/careers-path URL → tenant board,
      canonical collapses to the bare host)

## 2. Verification

- [x] 2.1 Confirm `internal/ingest/atsdetect`'s `TestLocalShapesStayOutsideTheSharedTable`
      still passes (guards that Paycom, Oracle, Taleo, NEOGOV, and Comeet stay outside the
      shared table)
- [x] 2.2 Run the full consumer set: `atsboard`, `boardresolve`, `linksource`, `contribution`,
      `atsdetect`
