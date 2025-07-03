package pushrequest

import (
	"context"
	"encoding/json"
	"fmt"
	"unsafe"

	"github.com/grafana/loki/pkg/push"
	"github.com/jmespath/go-jmespath"
	"github.com/tetratelabs/wazero/api"

	"github.com/grafana/loki/v3/pkg/plugins/host"
)

//go:generate go run ../bindgen/bindgen.go github.com/grafana/loki/v3/pkg/plugins/pushrequest PluginPushRequest
type PluginPushRequest interface {
	IterateLines(ctx context.Context, m api.Module, reqPtr uint64)
	ExtractFromJson(ctx context.Context, m api.Module, reqPtr uint64, streamIdx, entryIdx uint64, line string, nameToExpression ...string)
	AddStructuredMetadata(reqPtr uint64, streamIdx, entryIdx uint64, name, value string)
}

type HostPluginPushRequest struct {
	Exchange *host.Exchange
}

func (h HostPluginPushRequest) ExtractFromJson(ctx context.Context, m api.Module, reqPtr uint64, streamIdx, entryIdx uint64, line string, nameToExpression ...string) {
	if nameToExpression == nil || len(nameToExpression) == 0 {
		return
	}

	jsonMatchProcessor := m.ExportedFunction("process_json_match")
	if jsonMatchProcessor == nil {
		return
	}

	// We don't have the expected key -> expressions we don't know what's a key and what an expression, move on
	if (len(nameToExpression) % 2) != 0 {
		return
	}

	// Make sure the payload is valid json
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(line), &data); err != nil {
		return
	}

	// TODO we should register these in a setup so we don't compile them every time
	expressions := map[string]*jmespath.JMESPath{}
	for i := 0; i < len(nameToExpression); i += 2 {
		key := nameToExpression[i]
		value := nameToExpression[i+1]

		uncompiledExpression := key
		if value != "" {
			uncompiledExpression = value
		}

		expression, err := jmespath.Compile(uncompiledExpression)
		if err != nil {
			panic(fmt.Errorf("failed to compile JMESPath expression '%s': %w", uncompiledExpression, err))
		}
		expressions[key] = expression
	}

	stack := make([]uint64, 5)
	for key, expression := range expressions {
		value, err := expression.Search(data)
		if err != nil || value == nil {
			continue
		}

		valueStr := fmt.Sprintf("%v", value)

		stack[0] = reqPtr
		stack[1] = streamIdx
		stack[2] = entryIdx
		stack[3] = h.Exchange.PushString(m, key)
		stack[4] = h.Exchange.PushString(m, valueStr)

		err = jsonMatchProcessor.CallWithStack(ctx, stack)
		if err != nil {
			panic(fmt.Errorf("Failed to call process_json_match: %w\n", err))
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
