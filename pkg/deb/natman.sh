#!/bin/bash

# Networkd-dispatcher script for natman-go
# This file is placed in /etc/networkd-dispatcher/routable.d/natman.sh
# It triggers natman configuration when network interfaces become routable

set -e

# Configuration
NATMAN_BINARY="/usr/local/bin/natman"
NATMAN_CONFIG="/etc/natman/config.yaml"
LOG_TAG="natman-dispatcher"

# Logging function
log() {
    logger -t "$LOG_TAG" "$1"
    echo "$(date '+%Y-%m-%d %H:%M:%S') [natman-dispatcher] $1" >&2
}

# Check if natman is installed and configuration exists
if [ ! -x "$NATMAN_BINARY" ]; then
    log "natman binary not found at $NATMAN_BINARY"
    exit 0
fi

if [ ! -f "$NATMAN_CONFIG" ]; then
    log "natman configuration not found at $NATMAN_CONFIG"
    exit 0
fi

# Get interface name from environment
INTERFACE="${IFACE:-unknown}"

log "Interface $INTERFACE became routable, triggering natman configuration"

# Apply natman configuration with error handling
if "$NATMAN_BINARY" --quiet --c="$NATMAN_CONFIG" 2>&1 | logger -t "$LOG_TAG"; then
    log "natman configuration applied successfully for interface $INTERFACE"
else
    log "Failed to apply natman configuration for interface $INTERFACE"
    exit 1
fi