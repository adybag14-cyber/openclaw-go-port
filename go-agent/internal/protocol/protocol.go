package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

type FrameKind string

const (
	FrameKindReq     FrameKind = "req"
	FrameKindResp    FrameKind = "resp"
	FrameKindEvent   FrameKind = "event"
	FrameKindError   FrameKind = "error"
	FrameKindUnknown FrameKind = "unknown"
)

type MethodFamily string

const (
	MethodFamilyConnect  MethodFamily = "connect"
	MethodFamilyAgent    MethodFamily = "agent"
	MethodFamilySession  MethodFamily = "session"
	MethodFamilySessions MethodFamily = "sessions"
	MethodFamilyNode     MethodFamily = "node"
	MethodFamilyCron     MethodFamily = "cron"
	MethodFamilyGateway  MethodFamily = "gateway"
	MethodFamilyMessage  MethodFamily = "message"
	MethodFamilyBrowser  MethodFamily = "browser"
	MethodFamilyCanvas   MethodFamily = "canvas"
	MethodFamilyPairing  MethodFamily = "pairing"
	MethodFamilyConfig   MethodFamily = "config"
	MethodFamilyUnknown  MethodFamily = "unknown"
)

type RPCRequestFrame struct {
	ID     string
	Method string
	Params any
}

type RPCResponseFrame struct {
	ID     string
	OK     *bool
	Result any
	Error  *RPCErrorPayload
}

type RPCErrorPayload struct {
	Code    *int64
	Message string
	Details any
}

func ParseFrameText(text string) (map[string]any, error) {
	var frame map[string]any
	if err := json.Unmarshal([]byte(text), &frame); err != nil {
		return nil, err
	}
	return frame, nil
}

func FrameKindOf(frame map[string]any) FrameKind {
	if frame == nil {
		return FrameKindUnknown
	}

	switch strings.ToLower(strings.TrimSpace(stringField(frame, "type"))) {
	case "req":
		return FrameKindReq
	case "resp":
		return FrameKindResp
	case "event":
		return FrameKindEvent
	case "error":
		return FrameKindError
	default:
		if _, ok := frame["error"]; ok {
			return FrameKindError
		}
		return FrameKindUnknown
	}
}

func MethodName(frame map[string]any) string {
	return stringField(frame, "method")
}

func ClassifyMethod(method string) MethodFamily {
	normalized := strings.ToLower(strings.TrimSpace(method))
	if normalized == "connect" {
		return MethodFamilyConnect
	}
	if normalized == "health" || normalized == "status" {
		return MethodFamilyGateway
	}
	if strings.HasPrefix(normalized, "agent.") || normalized == "agent" ||
		strings.HasPrefix(normalized, "agents.") || normalized == "agents" {
		return MethodFamilyAgent
	}
	if strings.HasPrefix(normalized, "sessions.") || normalized == "sessions" {
		return MethodFamilySessions
	}
	if strings.HasPrefix(normalized, "session.") || normalized == "session" {
		return MethodFamilySession
	}
	if strings.HasPrefix(normalized, "node.") || normalized == "node" {
		return MethodFamilyNode
	}
	if strings.HasPrefix(normalized, "cron.") || normalized == "cron" {
		return MethodFamilyCron
	}
	if strings.HasPrefix(normalized, "gateway.") || normalized == "gateway" {
		return MethodFamilyGateway
	}
	if strings.HasPrefix(normalized, "usage.") || normalized == "usage" {
		return MethodFamilyGateway
	}
	if strings.HasPrefix(normalized, "models.") || normalized == "models" {
		return MethodFamilyGateway
	}
	if strings.HasPrefix(normalized, "skills.") || normalized == "skills" {
		return MethodFamilyGateway
	}
	if strings.HasPrefix(normalized, "update.") || normalized == "update" {
		return MethodFamilyGateway
	}
	if strings.HasPrefix(normalized, "web.") || normalized == "web" {
		return MethodFamilyGateway
	}
	if strings.HasPrefix(normalized, "wizard.") || normalized == "wizard" {
		return MethodFamilyGateway
	}
	if strings.HasPrefix(normalized, "message.") || normalized == "message" {
		return MethodFamilyMessage
	}
	if strings.HasPrefix(normalized, "browser.") || normalized == "browser" {
		return MethodFamilyBrowser
	}
	if strings.HasPrefix(normalized, "canvas.") || normalized == "canvas" {
		return MethodFamilyCanvas
	}
	if strings.HasPrefix(normalized, "pairing.") || normalized == "pairing" {
		return MethodFamilyPairing
	}
	if strings.HasPrefix(normalized, "config.") || normalized == "config" {
		return MethodFamilyConfig
	}
	return MethodFamilyUnknown
}

func ParseRPCRequest(frame map[string]any) *RPCRequestFrame {
	if FrameKindOf(frame) != FrameKindReq {
		return nil
	}
	method := MethodName(frame)
	if method == "" {
		return nil
	}

	id := "unknown"
	if value := stringField(frame, "id"); value != "" {
		id = value
	}

	params, ok := frame["params"]
	if !ok {
		params = nil
	}

	return &RPCRequestFrame{
		ID:     id,
		Method: method,
		Params: params,
	}
}

func ParseRPCResponse(frame map[string]any) *RPCResponseFrame {
	if FrameKindOf(frame) != FrameKindResp {
		return nil
	}

	id := "unknown"
	if value := stringField(frame, "id"); value != "" {
		id = value
	}

	var okPtr *bool
	if value, present := frame["ok"]; present {
		if b, castOK := value.(bool); castOK {
			okPtr = &b
		}
	}

	result, exists := frame["result"]
	if !exists {
		result = nil
	}

	return &RPCResponseFrame{
		ID:     id,
		OK:     okPtr,
		Result: result,
		Error:  ParseRPCError(frame),
	}
}

func ParseRPCError(frame map[string]any) *RPCErrorPayload {
	rawErr, ok := frame["error"]
	if !ok {
		return nil
	}
	errObj, ok := rawErr.(map[string]any)
	if !ok {
		return nil
	}

	message := "unknown error"
	if rawMessage, ok := errObj["message"].(string); ok {
		trimmed := strings.TrimSpace(rawMessage)
		if trimmed != "" {
			message = trimmed
		}
	}

	var codePtr *int64
	if rawCode, hasCode := errObj["code"]; hasCode {
		switch code := rawCode.(type) {
		case float64:
			v := int64(code)
			codePtr = &v
		case int64:
			v := code
			codePtr = &v
		case int:
			v := int64(code)
			codePtr = &v
		}
	}

	var details any
	if rawDetails, hasDetails := errObj["details"]; hasDetails {
		details = rawDetails
	}

	return &RPCErrorPayload{
		Code:    codePtr,
		Message: message,
		Details: details,
	}
}

func RPCSuccessResponseFrame(id string, result any) map[string]any {
	return map[string]any{
		"type":   "resp",
		"id":     id,
		"ok":     true,
		"result": result,
	}
}

func RPCErrorResponseFrame(id string, code int64, message string, details any) map[string]any {
	return map[string]any{
		"type": "resp",
		"id":   id,
		"ok":   false,
		"error": map[string]any{
			"code":    code,
			"message": message,
			"details": details,
		},
	}
}

func FrameRoot(frame map[string]any) map[string]any {
	if payload, ok := frame["payload"]; ok {
		if asMap, mapOK := payload.(map[string]any); mapOK {
			return asMap
		}
	}
	if params, ok := frame["params"]; ok {
		if asMap, mapOK := params.(map[string]any); mapOK {
			return asMap
		}
	}
	return frame
}

func FrameSource(frame map[string]any) string {
	if event := stringField(frame, "event"); event != "" {
		return event
	}
	if method := stringField(frame, "method"); method != "" {
		return method
	}
	return "gateway"
}

func stringField(frame map[string]any, key string) string {
	if frame == nil {
		return ""
	}
	raw, ok := frame[key]
	if !ok {
		return ""
	}
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		return fmt.Sprint(value)
	}
}
