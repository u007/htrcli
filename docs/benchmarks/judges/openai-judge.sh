#!/bin/sh
# OpenAI-compatible judge adapter for `htrcli bench run --judge-cmd`.
#
# Contract (see htrcli/internal/commands/bench_judge.go):
#   stdin : {"scenario":{...},"rubric":[{"id","weight","criterion"}],"evidence":{...}}
#   stdout: {"scores":[{"id","score","reason"}]}   with score in 0..1
#
# The runner computes the weighted total; this script only scores.
#
# Required environment (no defaults — a judge that guesses its endpoint is a
# judge you cannot audit):
#   JUDGE_URL     full URL of an OpenAI-compatible /v1/chat/completions endpoint
#   JUDGE_MODEL   model id to request
#   JUDGE_AUTH    "none" (llama-server, local no-auth) or "bearer" (needs JUDGE_API_TOKEN)
#   JUDGE_API_TOKEN  required only when JUDGE_AUTH=bearer; passed to curl via a
#                    config file so it never appears in argv or `ps`
#
# Requires jq on PATH and curl >= 7.76 (for --fail-with-body). The request body
# goes to curl on stdin (`-d @-`): a 32-step scenario approaches the macOS
# per-argument limit, so building it into argv is not safe.
set -eu

: "${JUDGE_URL:?set JUDGE_URL to an OpenAI-compatible chat/completions endpoint}"
: "${JUDGE_MODEL:?set JUDGE_MODEL to the model id}"
: "${JUDGE_AUTH:?set JUDGE_AUTH to 'none' or 'bearer'}"

case "$JUDGE_AUTH" in
none) ;;
bearer) : "${JUDGE_API_TOKEN:?set JUDGE_API_TOKEN when JUDGE_AUTH=bearer}" ;;
*)
	echo "judge: JUDGE_AUTH must be 'none' or 'bearer', got '$JUDGE_AUTH'" >&2
	exit 2
	;;
esac

if ! command -v jq >/dev/null 2>&1; then
	echo "judge: jq is required but was not found on PATH" >&2
	exit 2
fi

# Keep the WHOLE runner document: the evidence is what the judge grades, and a
# nested capture means the script cannot re-read stdin later.
doc=$(cat)
if [ -z "$doc" ]; then
	echo "judge: stdin is empty; expected the runner document" >&2
	exit 2
fi
if ! printf '%s' "$doc" | jq -e 'has("scenario") and has("rubric") and has("evidence")' >/dev/null; then
	echo "judge: stdin is not the runner document (needs .scenario, .rubric and .evidence)" >&2
	exit 2
fi

# Build the user message from all three parts. Sending only the rubric asks the
# model to invent a score: it would never see what the run actually did.
payload=$(printf '%s' "$doc" | jq -c --arg m "$JUDGE_MODEL" '{
	model: $m,
	temperature: 0,
	messages: [
		{
			role: "system",
			content: "You grade a browser agent against a rubric. Reply with JSON only, no prose and no code fences: {\"scores\":[{\"id\":\"<rubric id>\",\"score\":<number 0 to 1>,\"reason\":\"<short reason>\"}]}"
		},
		{
			role: "user",
			content: (
				"Score every rubric item against the run evidence.\n\nScenario:\n" + (.scenario | tojson)
				+ "\n\nRubric:\n" + (.rubric | tojson)
				+ "\n\nEvidence (the step results of the run):\n" + (.evidence | tojson)
			)
		}
	]
}')

body_file=$(mktemp)
cfg=""
cleanup() {
	rm -f "$body_file"
	if [ -n "$cfg" ]; then rm -f "$cfg"; fi
}
trap cleanup EXIT

# --fail-with-body is curl -f plus body retention: bare -f discards the
# response, which is exactly the diagnostic a 401 needs. Body on stdin, status
# captured so a non-2xx is reported with the server's own message.
if [ "$JUDGE_AUTH" = "bearer" ]; then
	cfg=$(mktemp)
	printf 'header = "Authorization: Bearer %s"\n' "$JUDGE_API_TOKEN" > "$cfg"
	set +e
	http_status=$(printf '%s' "$payload" | curl -sS --fail-with-body --max-time 90 --config "$cfg" \
		-H 'Content-Type: application/json' -d @- -o "$body_file" -w '%{http_code}' "$JUDGE_URL")
	curl_rc=$?
	set -e
else
	set +e
	http_status=$(printf '%s' "$payload" | curl -sS --fail-with-body --max-time 90 \
		-H 'Content-Type: application/json' -d @- -o "$body_file" -w '%{http_code}' "$JUDGE_URL")
	curl_rc=$?
	set -e
fi

if [ "$curl_rc" -ne 0 ]; then
	detail=""
	if [ -s "$body_file" ]; then detail=$(cat "$body_file"); fi
	echo "judge: request to $JUDGE_URL failed (curl exit $curl_rc, HTTP ${http_status:-000}): $detail" >&2
	exit 1
fi

response=$(cat "$body_file")
content=$(printf '%s' "$response" | jq -r '.choices[0].message.content')
if [ -z "$content" ] || [ "$content" = "null" ]; then
	echo "judge: response has no choices[0].message.content" >&2
	printf 'judge: response body: %s\n' "$response" >&2
	exit 1
fi

# A model may wrap its JSON in prose or fences. Pull the assistant message out
# of the chat envelope first, then extract the first JSON object from it, and
# fail loudly when there is none: a judge that cannot answer must not look like
# a judge that scored zero.
printf '%s' "$content" | jq -e -R -s '
	gsub("```json"; "") | gsub("```"; "")
	| (try fromjson catch (capture("(?s)(?<j>\\{.*\\})").j | fromjson))
	| if type == "object" and (.scores | type == "array")
	  then .
	  else error("judge model did not return a scores object")
	  end'
