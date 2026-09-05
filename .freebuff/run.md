# Live preview run doc — Warwick Institute (frontend)

Frontend-only preview: serves the React/Vite app (e.g. the student absence
form at `/absence`) with mocked-away API traffic. Backend Go service is NOT
required to view pages; `/api` calls proxy to `http://localhost:8080` and fail
loudly but harmlessly when no backend is up.

## 1. Reproduce the artifacts a fresh checkout needs

- Install dependencies with the project's package manager (npm):

  ```sh
  npm install
  ```

- No `.env` copying is needed to *serve* the frontend: `.env` holds backend /
  integration secrets (`DATABASE_URL`, `AUTH_PEPPER`, CRM creds, …) consumed by
  the Go server and test scripts, not by Vite. If you later run the backend,
  copy `.env` from the main checkout (never commit it — the file header says
  DO NOT COMMIT). `vite.config.ts` reads two optional vars: `VITE_API_TARGET`
  (proxy target for `/api`, default `http://localhost:8080`) and
  `VITE_SINGLE_FILE` (single-file bundle mode).
- Verify the toolchain: `node --version` (v26 works), `node_modules/.bin/vite`
  present after `npm install`.

## 2. Run the dev server (detached, survives the thread)

Preferred port: Vite's default **5173**. Check it is free first:
`lsof -nP -iTCP:5173 -sTCP:LISTEN`.

Plain `nohup` backgrounding gets reaped by the command runner in this
environment (process group killed when the tool call returns — the log stays
empty and the pid dies), so detach through **launchd** instead:

```sh
LOG="/ABS/PATH/TO/.freebuff/preview-<id>.log"   # ABSOLUTE path — launchd jobs
                                                 # start with cwd=/ and a relative
                                                 # redirect silently kills the job
launchctl submit -l com.freebuff.preview.absence-form -- /bin/sh -c \
  "cd '/ABS/PATH/TO/REPO' && exec /opt/homebrew/bin/node node_modules/vite/bin/vite.js > '$LOG' 2>&1"
```

Then confirm it is actually serving (not merely "spawn scheduled"):

```sh
launchctl print gui/$(id -u)/com.freebuff.preview.absence-form | grep -E 'pid =|state ='
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:5173/   # expect 200
```

Notes learned the hard way:

- **Absolute log path is mandatory.** The first attempts used a relative
  `.freebuff/…` path; launchd spawns `/bin/sh` with cwd `/`, the redirect
  failed, and the job exited 1 with an empty log (`launchctl print …` showed
  `last exit code = 1`, `runs` climbing). cd + absolute redirect fixes it.
- Use the absolute node path (`command -v node`); launchd jobs get a bare
  `PATH=/usr/bin:/bin:/usr/sbin:/sbin`, so bare `node`/`npm` won't resolve.
- Sanity test any doubt quickly:
  `launchctl submit -l com.freebuff.test.echo -- /bin/sh -c "echo hi > /tmp/x"`.

Teardown when done:

```sh
launchctl remove com.freebuff.preview.absence-form
```

App routes of interest: `/absence` (public student absence form),
`/` (login/app shell — needs the backend for data).
