package fileops

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
)

// FileOps package errors
var (
	ErrConfigFileRead         = errors.New("failed to read config file")
	ErrConfigFileParse        = errors.New("failed to parse config file")
	ErrMissingOutbounds       = errors.New("missing outbounds field in config")
	ErrInvalidOutboundsFormat = errors.New("invalid outbounds field format")
	ErrInsertMarkerNotFound   = errors.New("insert marker not found")
	ErrInvalidMarkerType      = errors.New("insert marker is not selector type")
	ErrConfigFileWrite        = errors.New("failed to write config file")
	ErrConfigSerialization    = errors.New("failed to serialize config")
)

// Updater provides configuration file updating functionality.
// It can update SingBox configuration files by inserting new proxy nodes
// at specified marker positions and managing selector outbound lists.
type Updater struct {
	insertMarker string
}

// NewUpdater creates a new configuration file updater with the specified insert marker.
// The insert marker is used to identify where new proxy nodes should be inserted.
func NewUpdater(insertMarker string) *Updater {
	return &Updater{
		insertMarker: insertMarker,
	}
}

// UpdateConfigFile updates the specified configuration file by inserting nodes at the marker position.
// Parameters:
//   - configPath: path to the configuration file to update
//   - nodes: list of proxy nodes to insert
//   - subscriptionNames: list of subscription names used to identify and clean old subscription nodes
func (u *Updater) UpdateConfigFile(configPath string, nodes []map[string]any, subscriptionNames []string) error {
	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Printf("%v %s: %v", ErrConfigFileRead, configPath, err)
		return fmt.Errorf("%w %s: %v", ErrConfigFileRead, configPath, err)
	}

	// 解析JSON配置
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		log.Printf("%v %s: %v", ErrConfigFileParse, configPath, err)
		return fmt.Errorf("%w %s: %v", ErrConfigFileParse, configPath, err)
	}

	// 检查outbounds字段
	outboundsRaw, ok := config["outbounds"]
	if !ok {
		log.Printf("%v: %s", ErrMissingOutbounds, configPath)
		return fmt.Errorf("%w in file %s", ErrMissingOutbounds, configPath)
	}

	outboundsArray, ok := outboundsRaw.([]any)
	if !ok {
		log.Printf("%v: %s", ErrInvalidOutboundsFormat, configPath)
		return fmt.Errorf("%w in file %s", ErrInvalidOutboundsFormat, configPath)
	}

	// 查找插入标记
	_, markerOutbound, err := u.findInsertMarker(outboundsArray)
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

// findInsertMarker locates the position of the insert marker in the outbounds array.
// It returns the index and the marker outbound configuration, or an error if not found.
func (u *Updater) findInsertMarker(outbounds []any) (int, map[string]any, error) {
	for i, outboundRaw := range outbounds {
		if outboundMap, ok := outboundRaw.(map[string]any); ok {
			if tag, ok := outboundMap["tag"].(string); ok && tag == u.insertMarker {
				return i, outboundMap, nil
			}
		}
	}
	return -1, nil, fmt.Errorf("%w: %s", ErrInsertMarkerNotFound, u.insertMarker)
}

// validateMarkerType validates that the insert marker is of selector type.
// Only selector type outbounds can be used as insert markers for proxy nodes.
func (u *Updater) validateMarkerType(markerOutbound map[string]any) error {
	markerType, ok := markerOutbound["type"].(string)
	if !ok || markerType != "selector" {
		return fmt.Errorf("%w: %s", ErrInvalidMarkerType, u.insertMarker)
	}
	return nil
}

// removeOldSubscriptionNodes removes old subscription nodes from the outbounds array.
// It identifies subscription nodes by checking if their tags contain subscription name prefixes.
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

// updateSelectorOutbounds updates the outbounds list of the selector marker.
// It removes old subscription node tags and adds new node tags to the selector's outbounds array.
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

// writeConfigFile writes the updated configuration back to the file.
// It serializes the configuration to JSON with proper indentation and writes it to disk.
func (u *Updater) writeConfigFile(configPath string, config map[string]any) error {
	updatedData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrConfigSerialization, err)
	}

	if err := os.WriteFile(configPath, updatedData, 0644); err != nil {
		return fmt.Errorf("%w %s: %v", ErrConfigFileWrite, configPath, err)
	}

	return nil
}
