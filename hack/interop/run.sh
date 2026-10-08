#!/bin/sh
# Executed inside hack/interop/Dockerfile; $1 is an empty state directory.
set -eu
S=$1
fail() { echo "INTEROP FAIL: $*"; exit 1; }
# Last drawn frame as compact [[r,g,b,l],...]
frame() { tail -n1 | jq -c '[.[]|[.r,.g,.b,.l]]'; }
Z='[0,0,0,0]'; G='[0,255,0,5]'; B='[0,0,255,5]'
f46="[$Z,$Z,$Z,$Z,$G,$Z,$B,$Z]"; f4="[$Z,$Z,$Z,$Z,$G,$Z,$Z,$Z]"; f6="[$Z,$Z,$Z,$Z,$Z,$Z,$B,$Z]"
dark="[$Z,$Z,$Z,$Z,$Z,$Z,$Z,$Z]"
check() { [ "$2" = "$3" ] || fail "$1: got $2 want $3"; echo "ok  $1"; }

check "go publishes px4"              "$(interop-go $S go-a publish 4 0 255 0 | frame)" "$f4"
check "rust sees go, adds px6"        "$(interop-rs $S rs-b publish 6 0 0 255 | frame)" "$f46"
check "go sees rust entry"            "$(interop-go $S go-a publish 4 0 255 0 | frame)" "$f46"
check "rust withdraw keeps go px4"    "$(interop-rs $S rs-b withdraw | frame)" "$f4"
check "go withdraw leaves dark"       "$(interop-go $S go-a withdraw | frame)" "$dark"
check "state empty after both"        "$(jq -c .owners $S/blinkt_state.json)" "{}"
check "rust writes, go reads"         "$(interop-rs $S rs-b publish 6 0 0 255 >/dev/null; interop-go $S go-a publish 4 0 255 0 | frame)" "$f46"
check "go writes, rust reads"         "$(interop-go $S go-a publish 4 0 255 0 >/dev/null; interop-rs $S rs-b publish 6 0 0 255 | frame)" "$f46"

# Shared flock: both hammer the file concurrently; neither entry may be lost.
interop-go $S go-a publish 4 0 255 0 200 >/dev/null & g=$!
interop-rs $S rs-b publish 6 0 0 255 200 >/dev/null & r=$!
wait $g; wait $r
check "concurrent go+rust keep both"  "$(jq -c '.owners|keys' $S/blinkt_state.json)" '["go-a","rs-b"]'
check "merged frame after contention" "$(interop-rs $S rs-b publish 6 0 0 255 | frame)" "$f46"
echo "INTEROP OK"
