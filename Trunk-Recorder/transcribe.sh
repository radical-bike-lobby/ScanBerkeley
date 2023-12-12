#!/bin/bash
set -eo pipefail

wav="$1"
json="$2"

if ! [[ -f $wav ]] || ! [[ -f $json ]]; then
  echo "Could not find file(s) $wav $json, make sure \`audioArchive\` and \`callLog\` are both set to true" >&2
  exit 1
fi

SYSTEM="$(jq -r '.short_name' $json)"
TALKGROUP="$(jq -r '.talkgroup' $json)"
FILENAME="$(basename $wav)"
FILEPATH="$SYSTEM/$TALKGROUP/$FILENAME"
# Call too short to be transcribed
if [[ "$(jq -r '.call_length' $json)" -lt "${MIN_CALL_LENGTH:-2}" ]]; then
  exit
fi

API_BASE_URL="https://trunk-transcribe.fly.dev "
# API_KEY="REDACTED_CHANGE_ME"

echo "Submitting $FILEPATH for transcription"
curl -v --connect-timeout 1 --form call_audio=@$wav --form call_json=@$json "$API_BASE_URL/transcribe"  &>/dev/null &
disown
# We run the curl command as a background process and disown it to not hang up trunk-recorder.
