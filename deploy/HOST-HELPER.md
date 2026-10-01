# The host helper

The Host page can restart the machine, shut it down, restart the
holzkube-manager service and install an update. It can also check for an
update without installing anything. holzkube-managerd itself
cannot do any of that, and is meant not to: it runs as an unprivileged user,
without capabilities, and starts no process. What it does instead is place a
one-line order in its data directory, `/var/lib/holzkube-manager/host-order`.

The **host helper** is what carries the order out. It is four files you
install by hand, as root, from the release archive:

- `holzkube-manager-host.path` -- a path unit. PID 1 watches for the order
  file; until one appears, nothing runs.
- `holzkube-manager-host.service` -- a oneshot service the path unit starts.
  It runs the script once per order, sandboxed as far as a reboot allows.
- `holzkube-manager-update-check.service` -- a oneshot the helper starts for
  a `check-update` order. It runs `/usr/local/sbin/holzkube-manager-update
  --check`, which looks up the newest release, records what it found, and
  installs nothing. It has a sandbox of its own: it may reach the network,
  which the helper's own service may not, and it writes only
  `/var/lib/holzkube-manager-update`. It also gets a system call filter
  (`@system-service`, which the helper's service cannot have, since a reboot
  needs more), sees in `/proc` no other user's processes and none of the
  kernel's settings, and has an IPC namespace of its own. The update script
  runs one run at a time: a check that finds the hourly update running waits
  for it, at most 60 s, and the hourly update waits for a running check, at
  most 600 s. A run that waited that long ends with exit code 1, records
  nothing, and says in its journal that another run held
  `/var/lib/holzkube-manager-update/.lock`.
- `holzkube-manager-host.sh`, installed as
  `/usr/local/sbin/holzkube-manager-host` -- the script. It removes the order
  before it does anything, and knows exactly five orders, each with one fixed
  command:

  | Order             | Command                                                |
  | ----------------- | ------------------------------------------------------ |
  | `reboot`          | `systemctl reboot`                                     |
  | `poweroff`        | `systemctl poweroff`                                   |
  | `restart-service` | `systemctl restart holzkube-manager.service`           |
  | `update`          | `systemctl start --no-block holzkube-manager-update.service` |
  | `check-update`    | `systemctl start holzkube-manager-update-check.service` |

  `check-update` waits for the check to end and records that end for the
  order: `done` when the check looked (what it found is in the update status
  the Host page shows), `failed` when it could not.

  Anything else -- a sixth word, a second line, a symlink, an order older than
  a minute -- is rejected. The text of an order is never executed, evaluated or
  written to the journal.

The daemon never gets root, and its own unit keeps every line of its hardening.
That is why the helper exists at all: the two obvious alternatives both needed
the daemon loosened. Asking systemd over D-Bus needs a Unix socket, which the
daemon's `RestrictAddressFamilies=` does not allow it. A sudoers entry needs a
process that may gain privileges, which its `NoNewPrivileges=true` forbids.
Here the daemon writes one file in a directory it already owns, and the
decision what to run stays with root's code.

Nothing installs the helper for you -- not the release, not the update script.
Putting new code on a host that runs as root is your decision, every time.

## What it needs

- systemd 250 or newer: from 250 on, a path unit whose service keeps failing
  stops itself instead of starting it forever.
- GNU coreutils (`dd` with `iflag=nofollow`, `stat`, `mktemp`).
- The daemon running as `holzkube-manager.service` with its data in
  `/var/lib/holzkube-manager` (otherwise see below).
- For both update orders, the update script at
  `/usr/local/sbin/holzkube-manager-update`, as the reference installation
  has it: `update` starts `holzkube-manager-update.service`, which runs it,
  and `check-update` starts `holzkube-manager-update-check.service`, which
  runs it with `--check`. The install commands below do not install it; see
  [The update script](#the-update-script). The other three orders do not
  need it.
- For `update`, also the unit it starts,
  `/etc/systemd/system/holzkube-manager-update.service`, which the release
  archive carries with its hourly timer; see
  [The hourly update](#the-hourly-update).

## Install

From the root of the unpacked release archive (or a checkout):

<!-- install-commands:begin -->
```sh
sudo install -o root -g root -m 0755 deploy/holzkube-manager-host.sh /usr/local/sbin/holzkube-manager-host
sudo install -o root -g root -m 0644 deploy/holzkube-manager-host.path deploy/holzkube-manager-host.service deploy/holzkube-manager-update-check.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now holzkube-manager-host.path
```
<!-- install-commands:end -->

These are the same four lines the Host page shows when the helper is missing.
The page checks for the script (owned by root and writable by nobody else),
the helper's two unit files, and the entry `systemctl enable` makes in
`/etc/systemd/system/paths.target.wants/`. Only the path unit is enabled: the
check unit has no `[Install]` section, and nothing but the helper starts it.

## The update script

**Check for updates** and **Check for updates and install** both end in
`/usr/local/sbin/holzkube-manager-update`, the script the hourly update runs.
The helper's install commands do not install it: it belongs to the update
mechanism, which replaces it itself after a healthy update. A machine set up
without it gets both update buttons switched off, with a note under the
page's header that names the file and shows the one command that installs it,
from the root of the unpacked release archive:

<!-- update-script-command:begin -->
```sh
sudo install -o root -g root -m 0755 deploy/holzkube-manager-update.sh /usr/local/sbin/holzkube-manager-update
```
<!-- update-script-command:end -->

The page counts it as installed when it is a regular file, executable, owned
by root and writable by nobody else -- the same test as for the helper's own
script, read from the file alone. Restart service, Restart host and Shut down
host do not need it and stay on. Until it is there, holzkube-manager refuses
both update actions with `409 conflict.host-update-script-missing` before it
issues a confirmation or places an order.

`update` also needs `holzkube-manager-update.service`, the unit the hourly
timer starts: see the next section.

## The hourly update

The release archive carries the two units that run the update script:

- `holzkube-manager-update.service` runs the script above, with no argument:
  it looks for a newer release, downloads and checks it, installs it,
  restarts holzkube-manager and goes back to the previous binary if the
  service does not come back healthy. **Check for updates and install**
  starts it, through the helper, and so does the timer. It runs as root,
  because it replaces root's binary and restarts the service, in a sandbox:
  it may write only the daemon binary's directory (`/usr/local/bin`), its own
  (`/usr/local/sbin`), the previous binary's
  (`/usr/local/lib/holzkube-manager`) and its status directory
  (`/var/lib/holzkube-manager-update`). On the network it needs GitHub and
  the health check on `127.0.0.1:8443`. Every line of the unit
  says in its comment why it is there, and which sandbox lines are left out
  and why.
- `holzkube-manager-update.timer` starts it 5 minutes after boot and then an
  hour after its last run, whoever started that run, each time with up to 5
  minutes of random delay.

Install both, from the root of the unpacked release archive, after the update
script's own command above:

<!-- update-unit-commands:begin -->
```sh
sudo install -d -o root -g root -m 0755 /usr/local/lib/holzkube-manager
sudo install -o root -g root -m 0644 deploy/holzkube-manager-update.service deploy/holzkube-manager-update.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now holzkube-manager-update.timer
```
<!-- update-unit-commands:end -->

The first line creates the directory the previous binary is kept in: the unit
lets the script write there, but cannot create it. Only the timer is enabled;
the service has no `[Install]` section.

These lines replace units of the same name that were written by hand. To see
what is there first:

```sh
systemctl cat holzkube-manager-update.service holzkube-manager-update.timer
```

Skipping the block keeps them as they are.

To see it work:

```sh
systemctl list-timers holzkube-manager-update.timer
journalctl -u holzkube-manager-update
```

The update script replaces the daemon and itself, never a unit. A newer unit
from a newer archive is taken by repeating the block above. To stop and remove
the hourly update:

```sh
sudo systemctl disable --now holzkube-manager-update.timer
sudo rm /etc/systemd/system/holzkube-manager-update.service /etc/systemd/system/holzkube-manager-update.timer
sudo systemctl daemon-reload
```

## Check that it works

1. Start the service once by hand, with no order waiting:

   ```sh
   sudo systemctl start holzkube-manager-host.service
   journalctl -u holzkube-manager-host -n 5
   ```

   The journal says `kein Auftrag` ("no order") and the service exits 0. That
   proves the sandbox starts and the script runs in it.

2. See that the path unit is waiting:

   ```sh
   systemctl status holzkube-manager-host.path
   ```

   It should say `active (waiting)`.

3. On the Host page, use **Check for updates** as the first real order. It
   installs nothing and takes nothing down, and goes the same way as the
   others -- the
   helper asks systemd to start a unit. The page reports what became of the
   order, and the status box then names the newest release.
   **Check for updates and install** can follow; the update service does the
   rest.

## The service's own unit stays as it is

Installing the helper asks for no change to `holzkube-manager.service`. Every
hardening line of the reference unit stays exactly as it is, in particular:

- `NoNewPrivileges=true`
- `CapabilityBoundingSet=` (empty)
- `RestrictAddressFamilies=AF_INET AF_INET6`
- `ProcSubset=pid`
- `ProtectProc=invisible`
- `ProtectSystem=strict`
- `StateDirectoryMode=0700`
- `UMask=0077`

The helper reaches into the daemon's data directory from its side, as root; the
daemon does not reach out to anything.

## A data directory other than /var/lib/holzkube-manager

The helper watches `/var/lib/holzkube-manager/host-order`. If the daemon runs
with a different data directory -- say `/srv/holzkube-manager` -- the helper
has to be told, in a drop-in for each unit. Without it every order waits for a
pickup that never comes and is withdrawn by the daemon after 10 seconds.

```sh
sudo systemctl edit holzkube-manager-host.path
```

```ini
[Path]
PathExists=
PathExists=/srv/holzkube-manager/host-order
```

The empty `PathExists=` clears the shipped path before the new one is set.

```sh
sudo systemctl edit holzkube-manager-host.service
```

```ini
[Service]
Environment=HOLZKUBE_MANAGER_HOST_ORDER=/srv/holzkube-manager/host-order
ReadWritePaths=-/srv/holzkube-manager
```

The `ReadWritePaths=` line lets the script take the order (a rename in that
directory) and remove it; the service is otherwise read-only on that path, and
an order it cannot take it does not carry out. `HOLZKUBE_MANAGER_HOST_ORDER` is
the one variable of the script meant to be set here; the script's other two
(`HOLZKUBE_MANAGER_HOST_STATE_DIR`, `HOLZKUBE_MANAGER_SYSTEMCTL`) exist for its
tests and are left alone.

## Results and reasons

- `/var/lib/holzkube-manager-host/last` holds one line about the last order:
  `<id> <order> started|done|rejected|failed <time>`. The Host page reads it.
  `done` comes only for `check-update`: the helper waits for the check unit
  and records its end, `done` when the unit ended well and `failed` when it
  did not. While a check's last record is `started`, the helper is still
  waiting for it and picks up no other order, and holzkube-manager refuses
  every host action meanwhile. A check left at `started` means the helper was
  ended while it waited: by its own 3-min limit, a reboot, or a kill.
- `journalctl -u holzkube-manager-host` says why: which order was started,
  why one was rejected (the reason and the length, never the text), or what
  failed.
- `journalctl -u holzkube-manager-update-check` says what the check itself
  did: the installed version, the newest release, or why it could not look.
- The script's exit code: `0` no order, started, or a check done; `1`
  failed; `2` rejected.

## When the path unit has stopped

If an order cannot be removed -- the data directory is read-only for the
service, or something other than a file sits under the order's name -- the
path unit starts the service again and again, and after five runs in ten
seconds systemd stops it for good: `systemctl status holzkube-manager-host.path`
shows `failed (Result: unit-start-limit-hit)`, and orders are no longer picked
up. Once the cause is fixed:

```sh
sudo systemctl reset-failed holzkube-manager-host.service holzkube-manager-host.path
sudo systemctl start holzkube-manager-host.path
```

## Updating

The helper is never updated automatically. The update script replaces the
daemon and itself, and never touches the helper's files. To take a newer
helper, repeat the install commands above from the newer archive.

The Host page says when the installed helper is older than holzkube-manager:
a helper installed before `check-update` existed carries out the four older
orders, and their buttons keep working, but **Check for updates** stays off,
and a note under the page's header names what is missing for it -- the script
that does not know `check-update`, the unit
`holzkube-manager-update-check.service`, or both -- with the same install
commands. Repeating them from the newer archive installs both, and the check
comes on with the next reading of the page.

## Uninstall

```sh
sudo systemctl disable --now holzkube-manager-host.path
sudo rm /usr/local/sbin/holzkube-manager-host /etc/systemd/system/holzkube-manager-host.path /etc/systemd/system/holzkube-manager-host.service /etc/systemd/system/holzkube-manager-update-check.service
sudo systemctl daemon-reload
```

`/var/lib/holzkube-manager-host` holds only the last result and can be removed
too. The Host page then says again that the helper is missing, and its buttons
stay off.
