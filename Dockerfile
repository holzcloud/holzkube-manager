# holzkube-manager as a container (OPS-04).
#
# Two things about this image are load-bearing rather than conventional, and
# both are about what a container cannot be allowed to make easier.
#
# It runs as a **non-root user**, and the data directory it writes is 0700
# owned by that user. holzkube-manager holds cluster PKI: a container that ran
# as root and wrote a world-readable volume would put every secret this product
# exists to protect one `docker cp` away from anybody on the host.
#
# It is built from **scratch**, not from alpine or distroless. The binary is
# static and embeds its own web assets, so it needs no shell and no package
# manager -- and every one of those would be a way in that this product has no
# use for.
#
# It *does* need a CA bundle, and the first version of this file claimed
# otherwise. The reasoning was that Talos endpoints are verified against the
# cluster PKI holzkube-manager holds rather than against a system trust store,
# which is true and is not the whole story: the Image Factory
# (`factory.talos.dev`) is a public HTTPS service, so the schematic and version
# routes verify against the public roots like any other client. Without the
# bundle the container starts, serves, manages nodes -- and answers every
# Factory route with `502 upstream.factory-unavailable`. Running the image is
# the only thing that would have caught that.

# --- the web bundle -------------------------------------------------------
#
# Built first and separately, because it changes for different reasons than the
# Go code and because `go build` needs its output embedded before it runs.
FROM node:22-alpine AS web

# /src/web, not /src, because vite.config.ts writes its bundle to
# `../internal/httpapi/dist` -- the path go:embed reads. The web stage therefore
# has to reproduce the repository's shape rather than flatten web/ to the root:
# built at the root, the bundle would land in `/internal/httpapi/dist`, which is
# how the first version of this file failed.
WORKDIR /src/web

# The lockfile alone first, so that a change to a .tsx file does not reinstall
# node_modules.
COPY web/package.json web/package-lock.json ./
# `npm ci`, not `npm install`: ci installs exactly what the lockfile says and
# fails if package.json disagrees, which is the property a reproducible image
# needs.
RUN npm ci

COPY web/ ./
RUN npm run build


# --- the binary -----------------------------------------------------------
FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# The web build's output, over whatever the repository had. internal/httpapi
# embeds this directory, so it has to exist before `go build` runs -- and the
# path is the one vite.config.ts writes, not a guess about where a bundle goes.
COPY --from=web /src/internal/httpapi/dist ./internal/httpapi/dist

ARG VERSION=dev

# CGO_ENABLED=0 is what makes a scratch image possible: the binary then has no
# dynamic loader to find and no libc to be missing.
#
# -trimpath so the paths in a panic are repository paths rather than this
# builder's directory layout, and so two builds of the same commit produce the
# same bytes.
RUN CGO_ENABLED=0 go build \
      -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /holzkube-managerd \
      ./cmd/holzkube-managerd

# The volume's mount point, created here because a scratch image has no shell
# to create it with. It has to exist in the image rather than be left to the
# volume: Docker seeds a fresh named volume from the image's directory,
# ownership included, and with nothing there it creates one owned by root --
# after which the non-root process cannot write its own lock file.
RUN mkdir -p /var/lib/holzkube-manager && chown 65532:65532 /var/lib/holzkube-manager


# --- the image ------------------------------------------------------------
FROM scratch

# The user and group the binary runs as. There is no /etc/passwd in a scratch
# image, so it is written here: a numeric USER alone works, and a container
# whose user has no name produces `whoami: unknown uid` in every diagnostic
# somebody runs while trying to work out why a volume is not writable.
# The public roots, for the Image Factory. See the note at the top of this file.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

COPY --from=build /etc/passwd /etc/passwd
COPY --chown=65532:65532 --from=build /holzkube-managerd /holzkube-managerd
COPY --from=build --chown=65532:65532 /var/lib/holzkube-manager /var/lib/holzkube-manager

# 65532 is the conventional "nonroot" uid, the same one distroless uses, so a
# volume prepared for one of those images works here without being re-chowned.
USER 65532:65532

# The data directory is a volume so that a container restart does not take the
# cluster PKI with it. It is declared rather than left implicit because a
# `docker run` without `-v` is otherwise an installation that silently loses
# everything on the next `docker rm`.
VOLUME ["/var/lib/holzkube-manager"]

# The data directory is a subdirectory of the volume, and that is not tidiness.
# Docker creates a volume's mount point at 0755 and copies only the image
# directory's *ownership* into it, not its mode -- so a data directory that
# *was* the volume root would be 0755, and the store refuses to open one that
# group or other can read. It refuses rather than quietly fixing it, which is
# the right behaviour and is why this is solved here instead: the process
# creates this subdirectory itself, at 0700, on first start.
ENV HOLZKUBE_MANAGER_DATA_DIR=/var/lib/holzkube-manager/data

# 0.0.0.0 and not 127.0.0.1: inside a container, loopback is reachable from
# nothing. That makes the loopback guard's plain-HTTP exemption inapplicable
# here, which is correct -- a container published to a network serves TLS or it
# does not serve.
ENV HOLZKUBE_MANAGER_LISTEN=0.0.0.0:8443

EXPOSE 8443

ENTRYPOINT ["/holzkube-managerd"]
