#!/usr/bin/env bash
# Reads one line from stdin, echoes it, then blocks until killed.
set -e

read -r line
echo "received: $line"
exec sleep 3600
