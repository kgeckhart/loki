package main

import (
	"github.com/grafana/loki/v3/pkg/plugins/guest"
	pushrequest "github.com/grafana/loki/v3/pkg/plugins/pushrequest/guest"
)

func main() {}

//export process_push_request
func ProcessPushRequest(reqPtr uint64) {
	pushrequest.IterateLines(reqPtr)
}

//export process_line
func ProcessLine(reqPtr, streamIdx, entryIdx uint64, linePtr uint64) {
	line := guest.ReadString(linePtr)

	if len(line) == 0 {
		return
	}

	pushrequest.AddStructuredMetadata(reqPtr, streamIdx, entryIdx, "new_metadata", "I added this")

	// Try to extract some metadata as json
	pushrequest.ExtractFromJson(reqPtr,
		streamIdx,
		entryIdx,
		line,
		"namespace", "meta.namespace",
		"route", "meta.req.route",
		"url", "meta.req.url",
		"requestId", "meta.requestId",
	)
}

//export process_json_match
func ProcessJsonMatch(reqPtr, streamIdx, entryIdx uint64, keyPtr uint64, valuePtr uint64) {
	key := guest.ReadString(keyPtr)
	value := guest.ReadString(valuePtr)

	pushrequest.AddStructuredMetadata(reqPtr, streamIdx, entryIdx, key, value)
}
