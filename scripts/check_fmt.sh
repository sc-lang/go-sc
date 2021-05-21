#!/bin/bash

files="$(shed run goimports -l .)"
if [ -n "$files" ]; then
    printf "Unformatted files found:\n%s\n" "$files"
    exit 1
fi
