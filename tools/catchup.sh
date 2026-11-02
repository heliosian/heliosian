#!/bin/sh
set -eu

printf '\nImporting school data\n'
go run ./tools/import

printf '\nFetching documents with the capture browser Google session\n'
go run ./tools/fetchsignin

printf '\nRendering missing PDF pages\n'
go run ./tools/pdfthumbnails

printf '\nCatch-up commands finished; check the fetch and PDF summaries for incomplete documents.\n'
