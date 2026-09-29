# The host helper

The Host page can restart the machine, shut it down, restart the
holzkube-manager service and install an update. holzkube-managerd itself
cannot do any of that, and is meant not to: it runs as an unprivileged user,
without capabilities, and starts no process. What it does instead is place a
one-line order in its data directory, `/var/lib/holzkube-manager/host-order`.

The **host helper** is what carries the order out. It is three files you
install by hand, as root, from the release archive:

- `holzkube-manager-host.path` -- a path unit. PID 1 watches for the order
  file; until one appears, nothing runs.
- `holzkube-manager-host.service` -- a oneshot service the path unit starts.
  It runs the script once per order, sandboxed as far as a reboot allows.
- `holzkube-manager-host.sh`, installed as
  `/usr/local/sbin/holzkube-manager-host` -- the script. It removes the order
  before it does anything, and knows exactly four orders, each with one fixed
  command:

  | Order             | Command                                                |
  | ----------------- | ------------------------------------------------------ |
  | `reboot`          | `systemctl reboot`                                     |
  | `poweroff`        | `systemctl poweroff`                                   |
  | `restart-service` | `systemctl restart holzkube-manager.service`           |
  | `update`          | `systemctl start --no-block holzkube-manager-update.service` |

  Anything else -- a fifth word, a second line, a symlink, an order older than
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
  `/var/lib/holzkube-manager` (otherwise see below), and, for the `update`
  order, `holzkube-manager-update.service`, the unit that runs the update
  script in the reference installation.

## Install

From the root of the unpacked release archive (or a checkout):

<!-- install-commands:begin -->
```sh
sudo install -o root -g root -m 0755 deploy/holzkube-manager-host.sh /usr/local/sbin/holzkube-manager-host
sudo install -o root -g root -m 0644 deploy/holzkube-manager-host.path deploy/holzkube-manager-host.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now holzkube-manager-host.path
```
<!-- install-commands:end -->

These are the same four lines the Host page shows when the helper is missing.
The page checks for the script (owned by root and writable by nobody else),
both unit files, and the entry `systemctl enable` makes in
`/etc/systemd/system/paths.target.wants/`.

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

3. On the Host page, use **Check for updates and install** as the first real
   order. It goes the same way as the other three -- the helper asks systemd
   to start a unit -- without taking the machine down. The page reports what
   became of the order, and the update service does the rest.

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
  `<id> <order> started|rejected|failed <time>`. The Host page reads it.
- `journalctl -u holzkube-manager-host` says why: which order was started,
  why one was rejected (the reason and the length, never the text), or what
  failed.
- The script's exit code: `0` no order, or started; `1` failed; `2` rejected.

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

## Uninstall

```sh
sudo systemctl disable --now holzkube-manager-host.path
sudo rm /usr/local/sbin/holzkube-manager-host /etc/systemd/system/holzkube-manager-host.path /etc/systemd/system/holzkube-manager-host.service
sudo systemctl daemon-reload
```

`/var/lib/holzkube-manager-host` holds only the last result and can be removed
too. The Host page then says again that the helper is missing, and its buttons
stay off.
