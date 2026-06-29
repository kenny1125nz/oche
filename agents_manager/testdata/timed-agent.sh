#!/usr/bin/env bash
# Reads a number of seconds from stdin, sleeps, then outputs done.
set -e

read -r seconds
sleep "$seconds"
echo "slept for ${seconds}s"
