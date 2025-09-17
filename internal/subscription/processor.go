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
		// relay 处理器需要特殊处理，在 manager 中创建
		return nil, fmt.Errorf("relay processor requires special initialization")
	default:
		return nil, fmt.Errorf("不支持的订阅类型: %s", subType)
	}
}
