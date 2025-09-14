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
	default:
		return nil, fmt.Errorf("不支持的订阅类型: %s", subType)
	}
}
