package fileops

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
)

// Updater 配置文件更新器
type Updater struct {
	insertMarker string
}

// NewUpdater 创建新的配置文件更新器
func NewUpdater(insertMarker string) *Updater {
	return &Updater{
		insertMarker: insertMarker,
	}
}

// UpdateConfigFile 更新指定的配置文件，将节点插入到标记位置
// configPath: 配置文件路径
// nodes: 要插入的节点列表
// subscriptionNames: 订阅名称列表，用于识别和清理旧的订阅节点
func (u *Updater) UpdateConfigFile(configPath string, nodes []map[string]any, subscriptionNames []string) error {
	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Printf("读取配置文件失败 %s: %v", configPath, err)
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	// 解析JSON配置
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		log.Printf("解析配置文件失败 %s: %v", configPath, err)
		return fmt.Errorf("解析配置文件失败: %v", err)
	}

	// 检查outbounds字段
	outboundsRaw, ok := config["outbounds"]
	if !ok {
		log.Printf("配置文件中缺少outbounds字段: %s", configPath)
		return fmt.Errorf("配置文件中缺少 outbounds 字段")
	}

	outboundsArray, ok := outboundsRaw.([]any)
	if !ok {
		log.Printf("outbounds字段格式错误: %s", configPath)
		return fmt.Errorf("outbounds 字段格式错误")
	}

	// 查找插入标记
	markerIndex, markerOutbound, err := u.findInsertMarker(outboundsArray)
	if err != nil {
		log.Printf("查找插入标记失败 %s: %v", configPath, err)
		return err
	}

	// 验证插入标记类型
	if err := u.validateMarkerType(markerOutbound); err != nil {
		log.Printf("插入标记验证失败 %s: %v", configPath, err)
		return err
	}

	// 清理旧的订阅节点
	newOutbounds := u.removeOldSubscriptionNodes(outboundsArray, subscriptionNames)

	// 添加新节点
	for _, node := range nodes {
		newOutbounds = append(newOutbounds, node)
	}

	// 更新selector的outbounds列表
	if err := u.updateSelectorOutbounds(newOutbounds, nodes, subscriptionNames); err != nil {
		log.Printf("更新selector outbounds失败 %s: %v", configPath, err)
		return err
	}

	// 更新配置
	config["outbounds"] = newOutbounds

	// 写回文件
	if err := u.writeConfigFile(configPath, config); err != nil {
		log.Printf("写入配置文件失败 %s: %v", configPath, err)
		return err
	}

	log.Printf("成功更新配置文件: %s", configPath)
	return nil
}

// findInsertMarker 查找插入标记的位置
func (u *Updater) findInsertMarker(outbounds []any) (int, map[string]any, error) {
	for i, outboundRaw := range outbounds {
		if outboundMap, ok := outboundRaw.(map[string]any); ok {
			if tag, ok := outboundMap["tag"].(string); ok && tag == u.insertMarker {
				return i, outboundMap, nil
			}
		}
	}
	return -1, nil, fmt.Errorf("未找到插入标记: %s", u.insertMarker)
}

// validateMarkerType 验证插入标记是否为selector类型
func (u *Updater) validateMarkerType(markerOutbound map[string]any) error {
	markerType, ok := markerOutbound["type"].(string)
	if !ok || markerType != "selector" {
		return fmt.Errorf("插入标记 %s 不是selector类型", u.insertMarker)
	}
	return nil
}

// removeOldSubscriptionNodes 移除旧的订阅节点
func (u *Updater) removeOldSubscriptionNodes(outbounds []any, subscriptionNames []string) []any {
	var newOutbounds []any

	for _, outboundRaw := range outbounds {
		if outboundMap, ok := outboundRaw.(map[string]any); ok {
			if tag, ok := outboundMap["tag"].(string); ok {
				isSubscriptionNode := false
				for _, subName := range subscriptionNames {
					if strings.Contains(tag, fmt.Sprintf("[%s]", subName)) {
						isSubscriptionNode = true
						break
					}
				}
				if !isSubscriptionNode {
					newOutbounds = append(newOutbounds, outboundRaw)
				}
			} else {
				newOutbounds = append(newOutbounds, outboundRaw)
			}
		} else {
			newOutbounds = append(newOutbounds, outboundRaw)
		}
	}

	return newOutbounds
}

// updateSelectorOutbounds 更新selector的outbounds列表
func (u *Updater) updateSelectorOutbounds(outbounds []any, nodes []map[string]any, subscriptionNames []string) error {
	// 收集新节点的标签
	var nodeTags []string
	for _, node := range nodes {
		if tag, ok := node["tag"].(string); ok {
			nodeTags = append(nodeTags, tag)
		}
	}

	// 找到并更新插入标记的outbounds列表
	for i, outboundRaw := range outbounds {
		if outboundMap, ok := outboundRaw.(map[string]any); ok {
			if tag, ok := outboundMap["tag"].(string); ok && tag == u.insertMarker {
				// 更新selector的outbounds列表
				if outboundList, ok := outboundMap["outbounds"].([]any); ok {
					// 移除旧的订阅节点标签
					var newOutboundList []any
					for _, tag := range outboundList {
						if tagStr, ok := tag.(string); ok {
							isSubscriptionTag := false
							for _, subName := range subscriptionNames {
								if strings.Contains(tagStr, fmt.Sprintf("[%s]", subName)) {
									isSubscriptionTag = true
									break
								}
							}
							if !isSubscriptionTag {
								newOutboundList = append(newOutboundList, tag)
							}
						} else {
							newOutboundList = append(newOutboundList, tag)
						}
					}
					// 添加新的节点标签
					for _, tag := range nodeTags {
						newOutboundList = append(newOutboundList, tag)
					}
					outboundMap["outbounds"] = newOutboundList
				} else {
					// 如果outbounds字段不存在，直接设置为节点标签数组
					var newOutboundList []any
					for _, tag := range nodeTags {
						newOutboundList = append(newOutboundList, tag)
					}
					outboundMap["outbounds"] = newOutboundList
				}
				outbounds[i] = outboundMap
				break
			}
		}
	}

	return nil
}

// writeConfigFile 将配置写回文件
func (u *Updater) writeConfigFile(configPath string, config map[string]any) error {
	updatedData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %v", err)
	}

	if err := os.WriteFile(configPath, updatedData, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %v", err)
	}

	return nil
}
