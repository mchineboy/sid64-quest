package game

import "encoding/json"

func propertyInt(props map[string]interface{}, key string) (int, bool) {
	if props == nil {
		return 0, false
	}
	value, ok := props[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	default:
		return 0, false
	}
}

func propertyBool(props map[string]interface{}, key string) bool {
	if props == nil {
		return false
	}
	value, ok := props[key]
	if !ok {
		return false
	}
	flag, ok := value.(bool)
	return ok && flag
}
