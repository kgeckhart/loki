package pushrequest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unsafe"

	"github.com/grafana/loki/pkg/push"
	"github.com/jmespath/go-jmespath"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/tetratelabs/wazero/api"

	"github.com/grafana/loki/v3/pkg/plugins/host"
)

//go:generate go run ../bindgen/bindgen.go github.com/grafana/loki/v3/pkg/plugins/pushrequest PluginPushRequest
type PluginPushRequest interface {
	IterateLines(ctx context.Context, m api.Module, reqPtr uint64)
	ExtractFromJson(ctx context.Context, m api.Module, reqPtr uint64, streamIdx, entryIdx uint64, line string, nameToExpression ...string)
	AddStructuredMetadata(reqPtr uint64, streamIdx, entryIdx uint64, name, value string)
	AddLabel(reqPtr uint64, streamIdx, entryIdx uint64, name, value string)
}

type HostPluginPushRequest struct {
	Exchange *host.Exchange
}

type ExtractType string

// TODO find an easier way to share this so we don't have guest/host duplication
const (
	ExtractTypeLabel    ExtractType = "label"
	ExtractTypeMetadata ExtractType = "metadata"
)

type ExpressionEntry struct {
	Type       ExtractType
	Key        string
	Expression *jmespath.JMESPath
}

func (h HostPluginPushRequest) ExtractFromJson(ctx context.Context, m api.Module, reqPtr uint64, streamIdx, entryIdx uint64, line string, typeNameExpression ...string) {
	if typeNameExpression == nil || len(typeNameExpression) == 0 {
		return
	}

	jsonLabelMatchProcessor := m.ExportedFunction("process_json_label_match")
	jsonMetadataMatchProcessor := m.ExportedFunction("process_json_metadata_match")

	if jsonLabelMatchProcessor == nil && jsonMetadataMatchProcessor == nil {
		return
	}

	// We need something divisible by 3, as we expect sets of type, name, expression
	if (len(typeNameExpression) % 3) != 0 {
		return
	}

	// Make sure the payload is valid json
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(line), &data); err != nil {
		return
	}

	// TODO we should register these in a setup so we don't compile them every time
	var expressions []ExpressionEntry
	for i := 0; i < len(typeNameExpression); i += 3 {
		extractType := ExtractType(typeNameExpression[i])
		key := typeNameExpression[i+1]
		value := typeNameExpression[i+2]

		if extractType != ExtractTypeLabel && extractType != ExtractTypeMetadata {
			panic(fmt.Errorf("invalid type '%s', only 'label' and 'metadata' are supported", extractType))
		}

		if extractType == ExtractTypeMetadata && jsonMetadataMatchProcessor == nil {
			panic(fmt.Errorf("metadata extractor registered with no metadata processor"))
		}

		if extractType == ExtractTypeLabel && jsonLabelMatchProcessor == nil {
			panic(fmt.Errorf("label extractor registered with no label processor"))
		}

		uncompiledExpression := key
		if value != "" {
			uncompiledExpression = value
		}

		expression, err := jmespath.Compile(uncompiledExpression)
		if err != nil {
			panic(fmt.Errorf("failed to compile JMESPath expression '%s': %w", uncompiledExpression, err))
		}
		expressions = append(expressions, ExpressionEntry{
			Type:       extractType,
			Key:        key,
			Expression: expression,
		})
	}

	stack := make([]uint64, 5)
	for _, entry := range expressions {
		value, err := entry.Expression.Search(data)
		if err != nil || value == nil {
			continue
		}
		valueStr := fmt.Sprintf("%v", value)

		stack[0] = reqPtr
		stack[1] = streamIdx
		stack[2] = entryIdx
		stack[3] = h.Exchange.PushString(m, entry.Key)
		stack[4] = h.Exchange.PushString(m, valueStr)

		var processor api.Function
		if entry.Type == ExtractTypeMetadata {
			processor = jsonMetadataMatchProcessor
		} else {
			processor = jsonLabelMatchProcessor
		}

		err = processor.CallWithStack(ctx, stack)
		if err != nil {
			panic(fmt.Errorf("Failed to call %s: %w\n", processor.Definition().Name(), err))
		}
	}

}

func (h HostPluginPushRequest) IterateLines(ctx context.Context, m api.Module, reqPtr uint64) {
	pushReq := (*push.PushRequest)(unsafe.Pointer(uintptr(reqPtr)))
	processLine := m.ExportedFunction("process_line")
	if processLine == nil {
		return
	}

	stack := make([]uint64, 4)
	for streamIdx, stream := range pushReq.Streams {
		for entryIdx, entry := range stream.Entries {
			stack[0] = reqPtr
			stack[1] = uint64(streamIdx)
			stack[2] = uint64(entryIdx)
			stack[3] = h.Exchange.PushString(m, entry.Line)

			err := processLine.CallWithStack(ctx, stack)
			if err != nil {
				panic(fmt.Errorf("Failed to call process_label: %w\n", err))
			}
		}
	}
}

func (h HostPluginPushRequest) AddStructuredMetadata(reqPtr uint64, streamIdx, entryIdx uint64, name, value string) {
	pushReq := (*push.PushRequest)(unsafe.Pointer(uintptr(reqPtr)))
	if len(pushReq.Streams) <= int(streamIdx) {
		panic(fmt.Errorf("invalid stream index %d, only %d streams available", streamIdx, len(pushReq.Streams)))
	}
	if len(pushReq.Streams[streamIdx].Entries) <= int(entryIdx) {
		panic(fmt.Errorf("invalid entry index %d for stream %d, only %d entries available", entryIdx, streamIdx, len(pushReq.Streams[streamIdx].Entries)))
	}

	stream := &pushReq.Streams[streamIdx]
	entry := &stream.Entries[entryIdx]

	if entry.StructuredMetadata == nil {
		entry.StructuredMetadata = make([]push.LabelAdapter, 0, 1)
	}

	entry.StructuredMetadata = append(entry.StructuredMetadata, push.LabelAdapter{
		Name:  name,
		Value: value,
	})
}

func (h HostPluginPushRequest) AddLabel(reqPtr uint64, streamIdx, entryIdx uint64, name, value string) {
	pushReq := (*push.PushRequest)(unsafe.Pointer(uintptr(reqPtr)))
	if len(pushReq.Streams) <= int(streamIdx) {
		panic(fmt.Errorf("invalid stream index %d, only %d streams available", streamIdx, len(pushReq.Streams)))
	}
	if len(pushReq.Streams[streamIdx].Entries) <= int(entryIdx) {
		panic(fmt.Errorf("invalid entry index %d for stream %d, only %d entries available", entryIdx, streamIdx, len(pushReq.Streams[streamIdx].Entries)))
	}

	stream := &pushReq.Streams[streamIdx]
	entry := &stream.Entries[entryIdx]

	// Parse existing labels and add the new label
	labelPairs := parseLabelsString(stream.Labels)
	labelPairs = append(labelPairs, name, value)

	// Create new labels from the pairs
	newLabels := labels.FromStrings(labelPairs...)
	newLabelsStr := newLabels.String()
	newLabelHash := newLabels.Hash()

	// This is the only entry in the stream, we can just update the labels
	if len(stream.Entries) == 1 {
		stream.Labels = newLabelsStr
		stream.Hash = newLabelHash
		return
	}

	// There are multiple entries we need to move the entry to a different stream, remove it from the current stream
	stream.Entries = append(stream.Entries[:entryIdx], stream.Entries[entryIdx+1:]...)

	// Check if there is another stream with the same labels, if so add the entry there
	for i, s := range pushReq.Streams {
		if i == int(streamIdx) {
			continue // Skip the current stream
		}

		if s.Labels == newLabelsStr {
			// Found a stream with the same labels, move the entry there
			s.Entries = append(s.Entries, *entry)
			return
		}
	}

	// We need a new stream with the new labels
	pushReq.Streams = append(pushReq.Streams, push.Stream{
		Labels:  newLabelsStr,
		Entries: []push.Entry{*entry},
		Hash:    newLabelHash,
	})
}

// parseLabelsString parses a labels string like "{job="test", instance="localhost"}"
// and returns a slice of name-value pairs suitable for labels.FromStrings()
func parseLabelsString(labelsStr string) []string {
	// Remove curly braces
	labelsStr = strings.Trim(labelsStr, "{}")
	var labelPairs []string

	if labelsStr != "" {
		// Split by comma and parse each label
		parts := strings.Split(labelsStr, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if equalIdx := strings.Index(part, "="); equalIdx > 0 {
				labelName := strings.TrimSpace(part[:equalIdx])
				labelValue := strings.Trim(strings.TrimSpace(part[equalIdx+1:]), "\"")
				labelPairs = append(labelPairs, labelName, labelValue)
			}
		}
	}

	return labelPairs
}
