#!/usr/bin/env sh

# 1. Configuration
REPO_SLUG="GhostWriters/DockSTARTer2"
TARGET_LIST="${HOME}/.local/bin:${HOME}/bin:/usr/local/bin:/usr/bin"
CHANNEL="$1"
CUSTOM_DEST="$2"
FILE_NAME="ds2"

# Conditional suffix assignment: if CHANNEL is set, SUFFIX is "-$CHANNEL"
# This aligns with your naming: ds2_VERSION-CHANNEL_OS_ARCH.tar.gz
SUFFIX=${CHANNEL:+"-${CHANNEL}"}

# 2. System Detection
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    armv7l|armhf) ARCH="arm" ;;
esac

# 3. Tool Detection
UA="User-Agent: ds2-installer"
if command -v curl >/dev/null 2>&1; then
    GET="curl -sL -H '$UA'"
elif command -v wget >/dev/null 2>&1; then
    GET="wget -qO- --header='$UA'"
else
    echo "Error: curl or wget required."
    exit 1
fi

# 4. Fetch Release Data and Find Asset URL
echo "Searching for $OS-$ARCH binary on GhostWriters/DockSTARTer2..."
JSON_DATA=$($GET "https://api.github.com/repos/${REPO_SLUG}/releases")

if [ -z "$JSON_DATA" ]; then
    echo "Error: Failed to fetch release data."
    exit 1
fi

DL_URL=""

# Robust POSIX split by common JSON delimiters
# We use a subshell to avoid messing with current shell's IFS for too long
# but here we just use it locally.
OLD_IFS="$IFS"
IFS=',{}[]'
for part in $JSON_DATA; do
    case "$part" in
        *\"browser_download_url\":*)
            URL=${part#*\"browser_download_url\":}
            URL=${URL#*\"}
            URL=${URL%\"*}
            
            # Precise Match: ds2_VERSION[-CHANNEL]_OS_ARCH.tar.gz
            # If CHANNEL is empty, we must ensure no extra hyphen/channel suffix exists.
            if [ -z "$CHANNEL" ]; then
                # Match: ds2_VERSION_OS_ARCH.tar.gz (VERSION has no hyphens)
                # We skip anything that looks like ds2_*-[channel]_OS_ARCH.tar.gz
                case "$URL" in 
                    *ds2_*-[!_]*_"$OS"_"$ARCH".tar.gz|*ds2_*-[!_]*_"$OS"_"$ARCH".tgz) continue ;;
                    *ds2_*"$OS"_"$ARCH".tar.gz|*ds2_*"$OS"_"$ARCH".tgz) ;;
                    *) continue ;;
                esac
            else
                # Match specific channel: ds2_VERSION-CHANNEL_OS_ARCH.tar.gz
                case "$URL" in
                    *ds2_*"$SUFFIX"_"$OS"_"$ARCH".tar.gz|*ds2_*"$SUFFIX"_"$OS"_"$ARCH".tgz) ;;
                    *) continue ;;
                esac
            fi
            
            # Additional check: Ignore auto-generated GitHub source code links
            case "$URL" in */archive/*) continue ;; esac
            DL_URL="$URL"
            break
            ;;
    esac
done
IFS="$OLD_IFS"

# 5. Validation
if [ -z "$DL_URL" ]; then
    echo "Error: No matching $OS-$ARCH asset found for channel: ${CHANNEL:-main}"
    exit 1
fi

# 6. Download and Extract via Pipe
TMP=$(mktemp -d 2>/dev/null || mktemp -d -t 'ds2')
echo "Download URL: $DL_URL"
$GET "$DL_URL" | tar -xzf - -C "$TMP"

# 7. Locate Binary
SRC_PATH=$(find "$TMP" -name "$FILE_NAME" -type f | head -n 1)
if [ -z "$SRC_PATH" ]; then
    echo "Error: '$FILE_NAME' not found in archive."
    rm -rf "$TMP"
    exit 1
fi
chmod +x "$SRC_PATH"

# is_on_path DIR: true if DIR (no trailing slash) appears in $PATH.
# Trailing slashes on PATH entries are stripped before comparing, so
# "/usr/bin/" in PATH still matches candidate "/usr/bin".
is_on_path() {
    OLD_IFS="$IFS"; IFS=":"
    for p in $PATH; do
        p=${p%/}
        if [ "$p" = "$1" ]; then
            IFS="$OLD_IFS"
            return 0
        fi
    done
    IFS="$OLD_IFS"
    return 1
}

# 8. Build Candidate List (Handles spaces via Positional Parameters)
if [ -n "$CUSTOM_DEST" ]; then
    set -- "$CUSTOM_DEST"
else
    EXISTING=$(command -v "$FILE_NAME")

    # Reorder TARGET_LIST to match PATH order (dirs on PATH tried first,
    # in PATH order), falling back to the rest of TARGET_LIST (not on
    # PATH) rather than failing outright if none of it is on PATH.
    # Mirrors DS1's symlink_ds.sh path_order().
    REMAINING="$TARGET_LIST"
    ORDERED=""
    OLD_IFS="$IFS"; IFS=":"
    for p in $PATH; do
        p=${p%/}
        [ -z "$p" ] && continue
        NEW_REMAINING=""
        FOUND=0
        for d in $REMAINING; do
            if [ "$FOUND" -eq 0 ] && [ "$d" = "$p" ]; then
                FOUND=1
                ORDERED="${ORDERED:+$ORDERED:}$d"
            else
                NEW_REMAINING="${NEW_REMAINING:+$NEW_REMAINING:}$d"
            fi
        done
        REMAINING="$NEW_REMAINING"
    done
    ORDERED="${ORDERED:+$ORDERED:}$REMAINING"
    IFS="$OLD_IFS"

    set --
    [ -n "$EXISTING" ] && set -- "$EXISTING"
    OLD_IFS="$IFS"; IFS=":"
    for d in $ORDERED; do
        [ -n "$d" ] && set -- "$@" "$d/$FILE_NAME"
    done
    IFS="$OLD_IFS"
fi

# 9. Installation Loop (Tries next folder on failure)
SUCCESS=0
for FINAL_DEST in "$@"; do
    case "$FINAL_DEST" in
        */*) DEST_DIR=${FINAL_DEST%/*} ;;
        *)   DEST_DIR="." ;;
    esac
    
    # Check accessibility: climb up to find first existing parent
    P="$DEST_DIR"
    while [ ! -d "$P" ] && [ "$P" != "/" ] && [ -n "$P" ]; do P=${P%/*}; done
    
    PRE=""
    [ ! -w "$P" ] && [ "$P" != "" ] && PRE="sudo"

    echo "Attempting to install to: $FINAL_DEST"
    
    if $PRE mkdir -p "$DEST_DIR" 2>/dev/null && \
       { [ ! -f "$FINAL_DEST" ] || $PRE mv -f "$FINAL_DEST" "$FINAL_DEST.old" 2>/dev/null; } && \
       $PRE cp "$SRC_PATH" "$FINAL_DEST" 2>/dev/null; then
        echo "Successfully installed to $FINAL_DEST"
        if ! is_on_path "$DEST_DIR"; then
            echo "Warning: '$DEST_DIR' is not in your PATH. Add it to your PATH to use the '$FILE_NAME' command."
        fi
        SUCCESS=1
        break
    else
        [ -n "$CUSTOM_DEST" ] && break
    fi
done

# Cleanup
rm -rf "$TMP"

if [ "$SUCCESS" -ne 1 ]; then
    echo "Error: All installation attempts failed."
    exit 1
fi

exit 0
