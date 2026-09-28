# Phase 13: Host-Aktionen über einen root-eigenen Helfer - Pattern Map

**Mapped:** 2026-09-29
**Files analyzed:** 22 (new + modified)
**Analogs found:** 20 / 22

All paths are repository-relative; every analog named here is git-tracked (`git ls-files` checked for `deploy/` and `internal/host/updatestatus/`).

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|---|---|---|---|---|
| `deploy/holzkube-manager-host.sh` (new) | root script | file-I/O, event-driven | `deploy/holzkube-manager-update.sh` | role-match |
| `deploy/holzkube-manager-host.path` (new) | config (systemd) | event-driven | none in repo (unit texts in RESEARCH Code Examples) | no analog |
| `deploy/holzkube-manager-host.service` (new) | config (systemd) | batch | none tracked (update unit lives only on the host) | no analog |
| `deploy/HOST-HELPER.md` (new) | docs | - | header comment of `deploy/holzkube-manager-update.sh` (lines 1-29) | partial |
| `internal/store/fsstore/atomic.go` (modify: `PlaceNew`, `Claim`) | utility (store primitive) | file-I/O | `writeAtomic` / `WriteFileAtomic` / `ReadFile` in same file | exact |
| `internal/host/hostaction/hostaction.go` (new: Action, Box, Place/withdraw/sweep) | service | file-I/O | `internal/history/persist.go` (`Open` with injected read/write) | role-match |
| `internal/host/hostaction/result.go` (new: strict `last` reader) | utility (parser) | file-I/O via fs.FS | `internal/host/updatestatus/status.go` (`Read`, `readLimited`, `parseTime`) | exact |
| `internal/host/hostaction/helper.go` (new: helper detection, `InstallCommands`) | utility | file-I/O via fs.FS | `internal/host/updatestatus/status.go` (`fs.Stat` + mode checks) | role-match |
| `internal/host/hostaction/script_test.go` (new) | test | integration (bash + stubs, unshare) | `internal/host/updatestatus/script_test.go` | exact |
| `internal/host/hostaction/units_test.go` (new) | test | batch (systemd-analyze) | `internal/depguard_test.go` (`goList` + external tool gate) | partial |
| `internal/host/hostaction/*_test.go` (place/withdraw/sweep/result/helper) | test | unit | `internal/host/updatestatus/status_test.go`, `internal/store/fsstore/atomic_test.go` | exact |
| `internal/host/host.go`, `collector.go` (modify: View gains `actions`) | model / collector | request-response | the `Update Reading[updatestatus.Status]` field (host.go:132-136, collector.go:115,165,215) | exact |
| `internal/httpapi/handlers/host.go` (modify: confirm + 4 action routes) | controller | request-response | `handlers/power.go:81-99` (route loop), `handlers/jobs.go:302-440` (confirm + nodeAction) | exact |
| `internal/httpapi/handlers/confirm_test.go` (modify + `TestEveryHostActionRequiresTyping`) | test | unit | same file lines 42-56, 135-147 | exact |
| `internal/httpapi/problem.go` (modify: 3 codes) | config (constants) | - | `CodePowerUnavailable`/`CodeExecRefused` (lines 140-153) | exact |
| `internal/audit/redact.go` (modify: 4 entries) | config (allowlist) | - | lines 219-230 (`node.reboot`, `action.confirm`) | exact |
| `internal/processguard_test.go` (new, R1 guard) | test | batch (go list + AST) | `internal/depguard_test.go:88-118` + AST scan in `internal/store/fsstore/permissions_test.go:247-276` | role-match |
| `internal/httpapi/hostapi_test.go` (modify: `TestHostActions`, `TestHostConfirm`) | test | integration | existing `hostapi_test.go`, `powerapi_test.go`, `jobsapi_test.go` | exact |
| `cmd/holzkube-managerd/main.go` (modify: wire `hostaction.NewBox`) | config (composition) | - | main.go:448 `history.Open(..., fsstore.ReadFile, fsstore.WriteFileAtomic, ...)` | exact |
| `docs/api-contract.md` (modify) | docs | - | existing entries; `contract_codes_test.go` enforces | exact |
| `web/src/components/HostActions.tsx` (+ `.test.tsx`) (new) | component | request-response | `web/src/components/NodeActions.tsx:119-242` (`RemoveFromClusterDialog`), `PowerMenu.tsx` (sudo/428) | exact |
| `web/src/api.ts` (modify: schema + `host.confirm`, `host.action`) | client | request-response | `api.ts:3382-3415` (`machines.confirm`, `removeFromCluster`), `api.ts:3131` (`host`) | exact |
| `web/src/routes/host.tsx` (+ `host.test.tsx`, `host.browser.test.tsx`) (modify) | route/page | polling | itself (notice stack, `useQuery` line 51) | exact |
| `web/fixtures/demo.json`, README, `docs/guide.md`, `internal/changelog/changelog.json` | docs/fixtures | - | existing host entries | exact |

## Pattern Assignments

### `internal/store/fsstore/atomic.go` - add `PlaceNew`, `Claim`

Copy the sequence of `writeAtomic` (lines 74-140) exactly, replacing only the final step:

```go
tmp, err := os.CreateTemp(dir, tempPrefix+"*")   // tempPrefix = store.TempFilePrefix (line 20): startup sweep finds orphans
defer func() { if err != nil && !crashed { _ = tmp.Close(); _ = os.Remove(tmpName) } }()
if err = tmp.Chmod(filePerm); err != nil { ... }  // 0600 BEFORE write -- Guard refuses group/other bits
... tmp.Write / tmp.Sync / tmp.Close ...
if err = os.Rename(tmpName, path); err != nil { ... }   // PlaceNew: os.Link(tmpName, path) instead; EEXIST -> fs.ErrExist; always remove tmp
if err = fsyncDir(dir); err != nil { ... }
```

Doc-comment style for exported wrappers (lines 158-181): `WriteFileAtomic` / `ReadFile` explain why they live in fsstore (`TestNoDirectFileAccessOutsideFsstore`). `Claim` = `os.Rename(path, dir/claimName)` then bounded read + `removeAndSync` (lines 152-158). Tests beside `atomic_test.go`.

### `internal/host/hostaction/hostaction.go` - Box (policy)

**Analog:** `internal/history/persist.go:95-101` - injection signature, no direct `os` file calls:

```go
func Open(
	path string,
	read func(path string) ([]byte, error),
	write func(path string, data []byte) error,
	now time.Time,
	logger *slog.Logger,
) *Store {
```

`NewBox(dataDir, place func(path string, data []byte) error, claim func(path, name string) ([]byte, error), ...)`; wired in `cmd/holzkube-managerd/main.go` next to line 448:

```go
historyStore := history.Open(history.Path(cfg.DataDir), fsstore.ReadFile, fsstore.WriteFileAtomic, time.Now(), logger)
```

### `internal/host/hostaction/result.go` - strict `last` reader

**Analog:** `internal/host/updatestatus/status.go:108-150, 196-210`. Copy verbatim structure:

```go
func Read(fsys fs.FS, path string) (Status, error) {
	name := strings.TrimPrefix(path, "/")
	if !fs.ValidPath(name) { ... }
	info, err := fs.Stat(fsys, name)
	if errors.Is(err, fs.ErrNotExist) { return Status{}, fmt.Errorf("%w (%s does not exist)", ErrNotRecorded, path) }
	if !info.Mode().IsRegular() { ... }
	if info.Size() > MaxSize { ... }
	raw, err := readLimited(fsys, name)   // io.LimitReader(f, MaxSize+1)
```

and the time rule (errors never quote the value):

```go
t, err := time.Parse(time.RFC3339, text)
if err != nil {
	// The parse error would quote the value; the rule is enough.
	return time.Time{}, errors.New("checked_at in the update status file is not an RFC 3339 time")
}
```

Replace the JSON decode with the one-line, four-field parse (`<id> <action> <result> <time>`, exact size, `-` id only with `rejected`).

### `internal/host/hostaction/helper.go` - detection

Same `fs.Stat` via `fs.FS` rooted at `/` as above; check script owner uid 0 (`info.Sys().(*syscall.Stat_t)` - keep behind the existing `sys_linux.go`/`sys_other.go` split in `internal/host`), mode not group/other-writable, executable; unit files present; `paths.target.wants/holzkube-manager-host.path` symlink present. Tests use `fstest.MapFS` like `status_test.go`.

### `internal/host/host.go` / `collector.go` - `actions` in the View

Mirror the `Update` field: `host.go:132-136` (`Update Reading[updatestatus.Status] \`json:"update"\``), default path in collector.go:115 (`cfg.UpdateStatusPath = updatestatus.DefaultPath`), hidden-for-role at collector.go:165 (`Hidden[...](r)`), read at collector.go:215 (`updatestatus.Read(c.cfg.FS, c.cfg.UpdateStatusPath)`).

### `internal/httpapi/handlers/host.go` - confirm + four actions

**Existing file shape** (host.go lines 1-60): long doc comment explaining each route setting, `HostRoutes(d) []httpapi.Route`, nil-dependency guard:

```go
if d.Host == nil {
	httpapi.WriteProblem(w, r, httpapi.Upstream("upstream.host-unavailable",
		"This instance was started without a host reader."))
	return
}
```

**Route loop** - copy `handlers/power.go:81-90`:

```go
httpapi.Route{
	Method:          http.MethodPost,
	Pattern:         "/api/v1/clusters/{id}/power/" + string(a),
	RequiresSession: true,
	MinRole:         model.RoleOperator,
	Destructive:     a.Sudo(),
	Action:          string(power.ClusterJobKind(a)),
	Handler:         handler(clusterPowerAction(d, a)),
},
```
(host: `Destructive: true`, `Action: "host." + string(a)`, no `ClusterScope`.)

**Confirm route** - copy `handlers/jobs.go:71-77` (not Destructive, `Action: "action.confirm"`) and `issueConfirmation` (jobs.go:302-383): decode body `{action, typed}`, look up a separate `hostTypedPhrase` (NOT `typedPhrase`, jobs.go:285-299), unknown action -> `httpapi.Validation(..., FieldError{Field: "action", ...})`, empty hostname -> 422, compare `strings.TrimSpace(body.Typed)` with fresh `Uname().Nodename` -> `FieldError{Field: "typed", Reason: "does not match the hostname"}`, then:

```go
intent := jobs.Intent{Action: body.Action, Machine: string(id), Params: body.Params}   // host: Machine: hostIntentTarget ("@host")
token, expires := d.Confirmer.Issue(intent)
writeJSON(w, http.StatusOK, map[string]any{"token": token, "expires": expires.Format(time.RFC3339), "action": body.Action, ...})
```
Do not call `jobsConfigured` (requires `d.Jobs`); guard `d.Confirmer` only.

**Action handler** - copy `nodeAction` (jobs.go:386-440): intent rebuilt from route, never from token:

```go
if err := d.Confirmer.Check(body.Confirmation, jobs.Intent{
	Action:  string(kind),
	Machine: string(id),
}); err != nil {
	writeJobError(w, r, d, err)   // jobs.go:445 -> 403 confirmation.invalid/expired
	return
}
...
writeJSON(w, http.StatusAccepted, map[string]any{ ... })   // 202: accepted, not happened
```
Order per RESEARCH Pattern 2: nil -> container 409 -> helper missing 409 -> Check -> Place (ErrExist -> 409 `conflict.host-order-pending`) -> 202 `{"order": ...}`.

### `internal/httpapi/problem.go` - three codes

Copy the comment-then-constant style (lines 140-153):

```go
// CodePowerUnavailable: one of the seven power actions cannot be pressed
// ... one sentence why it is a conflict and what the client shows.
CodePowerUnavailable = "conflict.power-unavailable"
```
Add `CodeHostOrderPending`, `CodeHostHelperMissing`, `CodeHostInContainer` as named constants (string literals are invisible to `TestEveryProblemCodeIsInTheContract`); add each to `docs/api-contract.md`.

### `internal/audit/redact.go` - allowlist

Next to lines 219-230:
```go
"node.reboot":   {"cluster"},
"node.shutdown": {"cluster"},
...
"action.confirm": {"action", "params.mode", "params.graceful", "params.reboot"},
```
Add `"host.reboot": {}, "host.poweroff": {}, "host.restart-service": {}, "host.update": {}` (empty lists, D-07). `typed` must never be added.

### `internal/httpapi/handlers/confirm_test.go`

Existing table walk (lines 42-56) must skip `host.*`; new `TestEveryHostActionRequiresTyping` walks `hostaction.Actions()` against `hostTypedPhrase` (all true, exactly four) and asserts disjointness from `typedPhrase`. The call-site scan (lines 135-147: "no Confirmer.Check call sites were found, so this test proves nothing") must learn that host call sites check `hostTypedPhrase`.

### `internal/processguard_test.go` (R1)

**Analog:** `internal/depguard_test.go:88-118` - `goList(t, "-deps", "./cmd/holzkube-managerd")` + pure classifier function + a negative-control test (`TestGuardRecognisesTheRootModule`). Use `-f '{{.ImportPath}} {{.Imports}}'`, filter own module prefix `github.com/holzcloud/holzkube-manager/`. AST half copies `internal/store/fsstore/permissions_test.go:247-276`:

```go
forbidden := map[string]bool{ "ReadFile": true, "WriteFile": true, "OpenFile": true, "Open": true, ... }
...
if pkg.Name == "ioutil" || (pkg.Name == "os" && forbidden[sel.Sel.Name]) {
```
with the selectors from R1 (`os.StartProcess`, `syscall.ForkExec`, `syscall.Exec`, `unix.Exec`, `SYS_EXECVE*`).

### `internal/host/hostaction/script_test.go`

**Analog:** `internal/host/updatestatus/script_test.go` - copy wholesale: `scriptEnv` with `write` helper (lines ~131-139), sealed `env()` (lines 144-160):

```go
// env is the whole environment of a run: nothing from the test process
// leaks in, so the host's own REPO, TMP or token never reach the script.
func (e *scriptEnv) env() []string {
	return append([]string{
		"PATH=" + e.stubs + ":/usr/local/bin:/usr/bin:/bin",
		"HOME=" + e.dir,
		"LC_ALL=C",
		"HOLZKUBE_MANAGER_UPDATE_STATUS_DIR=" + e.statusDir,
	}, e.extra...)
}
```
(host: `HOLZKUBE_MANAGER_HOST_ORDER`, `HOLZKUBE_MANAGER_HOST_STATE_DIR`, `HOLZKUBE_MANAGER_SYSTEMCTL`), `run(asRoot)` (lines 163-185) using `exec.Command("unshare", "--user", "--map-root-user", "bash", e.script)` and reading the exit code via `errors.As(err, &exitErr)`, and the unshare-availability skip (lines 516-528) before `TestUpdateScriptAsRoot` (line 530). Test files may use `os/exec`; the R1 guard scans non-test files only.

### `deploy/holzkube-manager-host.sh`

**Analog:** `deploy/holzkube-manager-update.sh` lines 1-60: German header comment (usage, what it does NOT touch), `set -euo pipefail`, `cd /` with the reason, and the documented override block:

```bash
# Pfade, die die Umgebung ueberschreiben darf. Im Betrieb setzt sie niemand:
# die Unit nicht, und sudo verwirft sie mit env_reset. Es gibt sie fuer den
# Test in internal/host/updatestatus, ...
#
#   HOLZKUBE_MANAGER_UPDATE_STATUS_DIR  /var/lib/holzkube-manager-update
```
Add `export LC_ALL=C`, `umask 022`, fixed `PATH`, EUID check, `dd iflag=nofollow,nonblock bs=65 count=1`, mtime age window (RESEARCH R-findings 3 and 5). The update script itself stays unchanged (D-19).

### `web/src/components/HostActions.tsx`

**Analog:** `web/src/components/NodeActions.tsx:119-242` (`RemoveFromClusterDialog`):

```tsx
const [typed, setTyped] = useState('')
const [failure, setFailure] = useState('')
const run = useMutation({
  mutationFn: async () => {
    const { token } = await api.machines.confirm(machine.id, 'node.remove-from-cluster', params, typed)
    return api.machines.removeFromCluster(machine.id, machine.cluster, token)
  },
  onSuccess: ... queryClient.invalidateQueries({ queryKey: [...] }),
  onError: (e: Error) => setFailure(e.message),
})
...
<Label htmlFor="remove-confirm">Type <span className="font-mono">{phrase}</span> to confirm</Label>
<Input id="remove-confirm" value={typed} autoComplete="off" onChange={(e) => setTyped(e.target.value)} />
<Button variant="destructive" disabled={typed !== phrase || run.isPending} onClick={() => run.mutate()}>
```
Notice boxes use the same `rounded-md border ... px-3 py-2` classes (amber/emerald/red at lines 180-209; UI-SPEC wants slate for information). Sudo/428 re-prompt: `PowerMenu.tsx` (doc at line 46, `a.sudo` at 197) with `SudoDialog.tsx`. Only `ui/` building blocks (UI-SPEC).

### `web/src/api.ts`

Copy `machines.confirm` (lines 3382-3392) and `removeFromCluster` (3405-3415):

```ts
confirm: (id: string, action: string, params: Record<string, string>, typed: string): Promise<{ token: string; expires: string }> =>
  sendJSON('POST', `/api/v1/machines/${encodeURIComponent(id)}/confirm`, confirmationSchema, { action, params, typed }),
```
Host: `host.confirm(action, typed)` -> `/api/v1/host/confirm`; `host.action(a, confirmation)` -> `/api/v1/host/actions/${a}` with a zod `order` schema. Extend `hostSchema` (near line 2603) with `actions` using `.default(...)` so an older daemon still parses (`api.nulls.test.ts`).

### `web/src/routes/host.tsx`

Uses the 3-s `useQuery` (line 51) - the action block renders from that response (UI-SPEC: no second poll). Imports block lines 1-24 show `@/` alias convention.

## Shared Patterns

### Gate chain on a route (auth, sudo, audit)
**Source:** `internal/httpapi/handlers/power.go:81-99`, `jobs.go:71-77`
**Apply to:** all five new routes. Declarative fields only (`RequiresSession`, `MinRole`, `Destructive`, `Action`); no per-handler auth code.

### Problem responses
**Source:** `internal/httpapi/problem.go` constants + `httpapi.Conflict(code, detail)`, `httpapi.Validation(msg, FieldError{...})`, `writeJobError` (jobs.go:445)
**Apply to:** handlers/host.go.

### File access only through fsstore
**Source:** `internal/store/fsstore/atomic.go:158-181`, guard `permissions_test.go:247-276`
**Apply to:** hostaction (daemon writes); reads of `/var/lib/holzkube-manager-host/last` and helper files go through the `fs.FS` rooted at `/` like `updatestatus`.

### Error text never quotes untrusted bytes
**Source:** `updatestatus/status.go:204-207`
**Apply to:** result.go, helper.go, and the shell script's journal lines.

### Guard seen red
Every new test (R1 guard, units gate, typed-phrase table, script rejections) gets its reinstated-fault run, as `depguard_test.go`'s negative control does.

## No Analog Found

| File | Role | Data Flow | Reason |
|---|---|---|---|
| `deploy/holzkube-manager-host.path` | systemd path unit | event-driven | no unit file is tracked in the repo; use RESEARCH Code Examples / Pattern 4 |
| `deploy/holzkube-manager-host.service` | systemd oneshot | batch | same; RESEARCH Pattern 4 lists every directive |
| `units_test.go` output-gated `systemd-analyze verify` | test | batch | no existing test drives systemd-analyze; use RESEARCH R7 sketch |

## Metadata

**Analog search scope:** `deploy/`, `internal/{host,httpapi,store/fsstore,history,audit}`, `internal/*_test.go`, `cmd/holzkube-managerd/main.go`, `web/src/{components,routes,api.ts}`
**Files scanned:** ~25
**Pattern extraction date:** 2026-09-29
