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

# Call too short to be transcribed
if [[ "$(jq -r '.call_length' $json)" -lt "${MIN_CALL_LENGTH:-2}" ]]; then
  exit
fi

file=$wav
bucket=scanner-berkeley
resource="/${bucket}/${file}"
contentType="binary/octet-stream"
dateValue=`date -R`
stringToSign="PUT\n\n${contentType}\n${dateValue}\n${resource}"
s3Key=AKIAYE233G7TVMFIBCMI
s3Secret=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
signature=`echo -en ${stringToSign} | openssl sha1 -hmac ${s3Secret} -binary | base64`
curl -L -X PUT -T "${file}" \
  -H "Host: ${bucket}.s3.amazonaws.com" \
  -H "Date: ${dateValue}" \
  -H "Content-Type: ${contentType}" \
  -H "Authorization: AWS ${s3Key}:${signature}" \
  https://${bucket}.s3.amazonaws.com/${file}

# API_BASE_URL="REDACTED_CHANGE_ME"
# API_KEY="REDACTED_CHANGE_ME"

# curl -s --connect-timeout 1 --request POST \
#     --url "$API_BASE_URL/tasks" \
#     --header "Authorization: Bearer $API_KEY" \
#     --header 'Content-Type: multipart/form-data' \
#     --form call_audio=@$wav \
#     --form call_json=@$json &>/dev/null &
disown
# We run the curl command as a background process and disown it to not hang up trunk-recorder.
