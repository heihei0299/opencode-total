package opencode

import (
	"encoding/json"
	"regexp"
	"strings"
)

var rAssignmentRegex = regexp.MustCompile(`\$R\[\d+\]\s*=\s*`)

type serovalInner struct {
	T int   `json:"t"`
	I int   `json:"i"`
	L int   `json:"l"`
	A []any `json:"a"`
}

type serovalEnvelope struct {
	T *serovalInner `json:"t"`
	F int           `json:"f"`
	M []any         `json:"m"`
}

func EncodePayload(args []any) string {
	env := serovalEnvelope{
		T: &serovalInner{
			T: 9,
			I: 0,
			L: len(args),
			A: args,
		},
		F: 31,
		M: []any{},
	}
	bytes, _ := json.Marshal(env)
	return string(bytes)
}

func DecodeStreamChunk(chunk string) (any, error) {
	text := strings.TrimSpace(chunk)
	if text == "" {
		return nil, nil
	}
	if strings.HasPrefix(text, "data:") {
		text = strings.TrimSpace(strings.TrimPrefix(text, "data:"))
	}

	var raw any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		// 尝试截取 JSON 部分
		firstBrace := strings.IndexAny(text, "{[")
		lastBrace := strings.LastIndexAny(text, "}]")
		if firstBrace != -1 && lastBrace > firstBrace {
			trimmed := text[firstBrace : lastBrace+1]
			if err2 := json.Unmarshal([]byte(trimmed), &raw); err2 != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	// 解包 Seroval 信封
	if m, ok := raw.(map[string]any); ok {
		if tVal, hasT := m["t"].(map[string]any); hasT {
			if aVal, hasA := tVal["a"].([]any); hasA {
				lVal, _ := tVal["l"].(float64)
				if lVal == 0 {
					return nil, nil
				}
				if lVal == 1 && len(aVal) > 0 {
					return aVal[0], nil
				}
				return aVal, nil
			}
		}
	}

	return raw, nil
}

func DecodeResponseText(text string) (any, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, nil
	}

	// 1. 处理 SolidStart 特有的 JS 分块 ($R[0]= 或 self.$R)
	if strings.Contains(trimmed, "self.$R") || strings.Contains(trimmed, "$R[0]=") {
		target := trimmed
		if idx := strings.Index(target, "$R[0]="); idx != -1 {
			target = target[idx+len("$R[0]="):]
		}
		// 清除自调用后缀例如 )($R)
		if idx := strings.Index(target, ")($R"); idx != -1 {
			target = target[:idx]
		}
		// 截取 JSON 结构部分
		firstBrace := strings.IndexAny(target, "{[")
		lastBrace := strings.LastIndexAny(target, "}]")
		if firstBrace != -1 && lastBrace >= firstBrace {
			jsonSlice := target[firstBrace : lastBrace+1]
			// 移除所有 $R[N]= 赋值语句，转换为标准 JSON
			cleanJson := rAssignmentRegex.ReplaceAllString(jsonSlice, "")

			var parsed any
			if err := json.Unmarshal([]byte(cleanJson), &parsed); err == nil {
				return parsed, nil
			}
		}
	}

	// 2. 普通 JSON / Seroval 信封
	return DecodeStreamChunk(trimmed)
}
