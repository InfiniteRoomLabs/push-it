#!/bin/sh
# Builds the universal macOS glow helper into internal/glow/macos/bin/glow-macos.
set -eu
cd "$(dirname "$0")"
mkdir -p bin
swiftc -O -target arm64-apple-macos12 -o bin/glow-macos-arm64 glow.swift
swiftc -O -target x86_64-apple-macos12 -o bin/glow-macos-x86_64 glow.swift
lipo -create -output bin/glow-macos bin/glow-macos-arm64 bin/glow-macos-x86_64
rm -f bin/glow-macos-arm64 bin/glow-macos-x86_64
./bin/glow-macos --dry-run
echo "built bin/glow-macos ($(lipo -archs bin/glow-macos))"
