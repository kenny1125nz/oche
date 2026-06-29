#!/usr/bin/env bash
# Dummy agent: reads a line from stdin, echoes it back line by line.
# Used for pool integration tests.
set -e

if [ -t 0 ]; then
    echo "Usage: echo <input> | $0"
    exit 1
fi

while IFS= read -r line; do
    echo "received: $line"
    sleep 0.1
done
