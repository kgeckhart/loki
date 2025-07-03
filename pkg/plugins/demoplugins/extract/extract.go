package main

import (
	"strings"

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

	if len(line) == 0 || !strings.Contains(line, "extract-me") {
		return
	}

	pushrequest.ExtractFromJson(reqPtr,
		streamIdx,
		entryIdx,
		line,
		// Hard-coded examples for now, we could have these be injected through tenant-based configuration
		string(pushrequest.ExtractTypeLabel), "namespace", "meta.namespace",
		string(pushrequest.ExtractTypeLabel), "route", "meta.req.route",
		string(pushrequest.ExtractTypeMetadata), "url", "meta.req.url",
		string(pushrequest.ExtractTypeMetadata), "requestId", "meta.requestId",
	)
}

//export process_json_metadata_match
func ProcessJsonMetadataMatch(reqPtr, streamIdx, entryIdx uint64, keyPtr uint64, valuePtr uint64) {
	key := guest.ReadString(keyPtr)
	value := guest.ReadString(valuePtr)

	pushrequest.AddStructuredMetadata(reqPtr, streamIdx, entryIdx, key, value)
}

//export process_json_label_match
func ProcessJsonLabelMatch(reqPtr, streamIdx, entryIdx uint64, keyPtr uint64, valuePtr uint64) {
	key := guest.ReadString(keyPtr)
	value := guest.ReadString(valuePtr)

	pushrequest.AddLabel(reqPtr, streamIdx, entryIdx, key, value)
}
