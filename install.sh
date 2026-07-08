#!/usr/bin/env bash
# shellcheck shell=bash
set -euo pipefail

# -----------------------------------------------------------------------------
# sdev one-line network installer / self-update engine.
#
# Fresh install (mac/Linux):
#   curl -fsSL https://raw.githubusercontent.com/santhosh2011/sdev/main/install.sh | bash
#
# This script downloads the latest published sdev release zip, verifies its
# SHA-256, unpacks it, and hands off to the bundled ./install (which places the
# tool, wires your shell, and preserves your $SDEV_HOME data). It is also the
# engine behind `sdev update` / `sdev-update`.
#
# IMPORTANT: `curl … | bash` runs under macOS's system bash (3.2), so this file
# must stay POSIX/bash-3.2 compatible. It only *checks for* and hands off to a
# bash >= 4 for the actual install work.
#
# Env knobs (all optional):
#   SDEV_REPO         owner/repo to fetch from   (default: santhosh2011/sdev)
#   SDEV_VERSION      tag/version to install      (default: latest release)
#   SDEV_DIST_ZIP     use a local zip, skip fetch (offline / tests / CI)
#   SDEV_HOME / SDEV_INSTALL / SDEV_BIN_DIR / SDEV_CLAUDE
#                     passed through untouched to the bundled ./install
# -----------------------------------------------------------------------------

REPO="${SDEV_REPO:-santhosh2011/sdev}"

say()  { printf '%s\n' "$*"; }
warn() { printf '%s\n' "$*" >&2; }
die()  { warn "✗ $*"; exit 1; }

# --- 0. platform gate -------------------------------------------------------
os="$(uname -s 2>/dev/null || echo unknown)"
case "$os" in
    Darwin|Linux) : ;;
    *) die "sdev supports macOS and Linux only (found '$os')." ;;
esac

# --- 1. locate a bash >= 4 to run the bundled installer ---------------------
# curl|bash may run us under bash 3.2; the tool itself needs bash >= 4.
find_bash4() {
    local cand ver
    for cand in "${BASH:-}" bash /opt/homebrew/bin/bash /usr/local/bin/bash /usr/bin/bash; do
        [ -n "$cand" ] || continue
        command -v "$cand" >/dev/null 2>&1 || { [ -x "$cand" ] || continue; }
        # shellcheck disable=SC2016  # single quotes are intentional: evaluated by the child bash
        ver="$("$cand" -c 'printf %s "${BASH_VERSINFO[0]:-0}"' 2>/dev/null || echo 0)"
        if [ "${ver:-0}" -ge 4 ] 2>/dev/null; then
            command -v "$cand" 2>/dev/null || printf '%s' "$cand"
            return 0
        fi
    done
    return 1
}
BASH4="$(find_bash4 || true)"
if [ -z "$BASH4" ]; then
    warn "✗ bash >= 4 required (found ${BASH_VERSION:-?})."
    if [ "$os" = Darwin ]; then warn "   macOS ships bash 3.2 — install a newer one: brew install bash"
    else warn "   Linux: install a newer bash via your package manager"; fi
    exit 1
fi
# shellcheck disable=SC2016  # single quotes are intentional: evaluated by the child bash
say "✓ bash $("$BASH4" -c 'printf %s "${BASH_VERSINFO[0]}"') ($BASH4)"

# --- 2. yq must be mikefarah v4, NOT the Python yq --------------------------
if ! command -v yq >/dev/null 2>&1; then
    warn "✗ yq (mikefarah) v4 required and not found."
    if [ "$os" = Darwin ]; then warn "   macOS: brew install yq"
    else warn "   Linux: see https://github.com/mikefarah/yq#install"; fi
    exit 1
fi
yq_ver="$(yq --version 2>&1 || true)"
case "$yq_ver" in
    *mikefarah*) : ;;
    *) die "wrong yq detected ('$yq_ver'). sdev needs mikefarah yq v4, not the Python yq — see https://github.com/mikefarah/yq#install" ;;
esac
yq_major="$(printf '%s' "$yq_ver" | grep -oE 'version v?[0-9]+' | grep -oE '[0-9]+' | head -1)"
[ "${yq_major:-0}" -ge 4 ] 2>/dev/null || die "yq v4+ required (found '$yq_ver'). Upgrade: brew install yq (macOS) / https://github.com/mikefarah/yq#install"
say "✓ yq ($yq_ver)"

# --- 3. small download / checksum helpers -----------------------------------
fetch_to() {  # fetch_to <url> <dest-file>
    local url="$1" dest="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$url" -o "$dest"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "$dest" "$url"
    else
        die "need curl or wget to download."
    fi
}

fetch_stdout() {  # fetch_stdout <url>
    local url="$1"
    if command -v curl >/dev/null 2>&1; then curl -fsSL "$url"
    elif command -v wget >/dev/null 2>&1; then wget -qO- "$url"
    else die "need curl or wget to download."; fi
}

verify_sha256() {  # verify_sha256 <sha-file> ; run from the file's dir. nonzero ONLY on a real mismatch.
    if command -v shasum >/dev/null 2>&1; then
        shasum -a 256 -c "$1" || return 1
    elif command -v sha256sum >/dev/null 2>&1; then
        sha256sum -c "$1" || return 1
    else
        warn "⚠️  no shasum/sha256sum — skipping checksum verification."
        return 0
    fi
    say "✓ checksum verified"
}

# --- 4. resolve the release + acquire the zip -------------------------------
TMP="$(mktemp -d)"
# shellcheck disable=SC2064
trap "rm -rf '$TMP'" EXIT

ZIP=""
if [ -n "${SDEV_DIST_ZIP:-}" ]; then
    [ -f "$SDEV_DIST_ZIP" ] || die "SDEV_DIST_ZIP='$SDEV_DIST_ZIP' not found."
    ZIP="$SDEV_DIST_ZIP"
    say "✓ using local zip $ZIP"
    if [ -f "$ZIP.sha256" ]; then
        ( cd "$(dirname "$ZIP")" && verify_sha256 "$(basename "$ZIP").sha256" ) \
            || die "checksum verification failed for $ZIP — refusing to install a corrupted archive."
    fi
else
    # Resolve the target tag: explicit SDEV_VERSION, else the latest release.
    tag="${SDEV_VERSION:-}"
    if [ -z "$tag" ]; then
        say "→ resolving latest sdev release from $REPO…"
        api="$(fetch_stdout "https://api.github.com/repos/$REPO/releases/latest" || true)"
        if command -v jq >/dev/null 2>&1; then
            tag="$(printf '%s' "$api" | jq -r '.tag_name // empty' 2>/dev/null || true)"
        fi
        [ -n "$tag" ] || tag="$(printf '%s' "$api" | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/')"
        [ -n "$tag" ] || die "could not resolve the latest release tag from $REPO (no releases published yet?). Set SDEV_VERSION or SDEV_DIST_ZIP."
    fi
    # Tags are vX.Y.Z; the zip asset is sdev-X.Y.Z.zip.
    ver="${tag#v}"
    base="https://github.com/$REPO/releases/download/$tag"
    say "→ downloading sdev $tag…"
    fetch_to "$base/sdev-$ver.zip" "$TMP/sdev-$ver.zip"
    if fetch_to "$base/sdev-$ver.zip.sha256" "$TMP/sdev-$ver.zip.sha256" 2>/dev/null; then
        ( cd "$TMP" && verify_sha256 "sdev-$ver.zip.sha256" ) \
            || die "checksum verification failed for sdev-$ver.zip — refusing to install a corrupted download."
    else
        warn "⚠️  no published checksum for $tag — skipping verification."
    fi
    ZIP="$TMP/sdev-$ver.zip"
fi

# --- 5. unpack + hand off to the bundled installer --------------------------
command -v unzip >/dev/null 2>&1 || die "need 'unzip' to unpack the release."
UNPACK="$TMP/unpack"
mkdir -p "$UNPACK"
unzip -q "$ZIP" -d "$UNPACK"
# dist ships the tool under a top-level sdev/ directory.
SRC="$UNPACK/sdev"
if [ ! -f "$SRC/install" ]; then
    # Fallback: single top-level directory inside the archive.
    SRC="$(cd "$UNPACK" && for d in */; do printf '%s' "${d%/}"; break; done)"
    SRC="$UNPACK/$SRC"
fi
[ -f "$SRC/install" ] || die "release archive is missing ./install."

say "→ running the bundled installer…"
chmod +x "$SRC/install" 2>/dev/null || true
exec "$BASH4" "$SRC/install"
