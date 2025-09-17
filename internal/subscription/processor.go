package subscription

import (
	"fmt"
	"strings"
)

// NewProcessor 根据订阅类型创建相应的处理器
func NewProcessor(subType string) (Processor, error) {
	switch strings.ToLower(subType) {
	case "clash":
		return NewClashProcessor(), nil
	case "singbox":
		return NewSingBoxProcessor(), nil
	case "relay":
		// relay 类型与 singbox 完全一致，使用相同的处理器
		return NewSingBoxProcessor(), nil
	default:
		return nil, fmt.Errorf("不支持的订阅类型: %s", subType)
	}
}
