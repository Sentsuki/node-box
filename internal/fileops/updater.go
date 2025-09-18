package fileops

import (
	"encoding/json"
	"errors"
	"fmt"
	"node-box/internal/logger"
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

// CleanSubscriptionArtifacts removes any nodes (outbounds) and selector tags
// that contain any of the given subscription names (identified by "[name]").
// This is used at the beginning of an update cycle to ensure previously added
// but now excluded content is fully cleaned up.
func (u *Updater) CleanSubscriptionArtifacts(configPath string, subscriptionNames []string) error {
	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("%w %s: %v", ErrConfigFileRead, configPath, err)
	}

	// 解析JSON配置
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("%w %s: %v", ErrConfigFileParse, configPath, err)
	}

	// 检查outbounds字段
	outboundsRaw, ok := config["outbounds"]
	if !ok {
		return fmt.Errorf("%w in file %s", ErrMissingOutbounds, configPath)
	}

	outboundsArray, ok := outboundsRaw.([]any)
	if !ok {
		return fmt.Errorf("%w in file %s", ErrInvalidOutboundsFormat, configPath)
	}

	containsSub := func(s string) bool {
		for _, subName := range subscriptionNames {
			if strings.Contains(s, fmt.Sprintf("[%s]", subName)) {
				return true
			}
		}
		return false
	}

	var newOutbounds []any
	for _, outboundRaw := range outboundsArray {
		outboundMap, ok := outboundRaw.(map[string]any)
		if !ok {
			// 非对象，直接保留
			newOutbounds = append(newOutbounds, outboundRaw)
			continue
		}

		// 如果是带有 tag 的节点并且包含任一订阅名，整条节点删除
		if tag, ok := outboundMap["tag"].(string); ok && tag != "" {
			if containsSub(tag) {
				continue
			}
		}

		// 如果是 selector，清理其 outbounds 列表里包含订阅名的 tag
		if t, ok := outboundMap["type"].(string); ok && t == "selector" {
			if obList, ok := outboundMap["outbounds"].([]any); ok {
				var filtered []any
				for _, ob := range obList {
					if s, ok := ob.(string); ok {
						if containsSub(s) {
							continue
						}
					}
					filtered = append(filtered, ob)
				}
				outboundMap["outbounds"] = filtered
			}
		}

		newOutbounds = append(newOutbounds, outboundMap)
	}

	// 更新配置
	config["outbounds"] = newOutbounds

	// 写回文件
	if err := u.writeConfigFile(configPath, config); err != nil {
		return err
	}

	logger.Debug("清理订阅残留: %s", configPath)
	return nil
}

// cloneMap creates a shallow copy of a map[string]any
func cloneMap(m map[string]any) map[string]any {
	c := make(map[string]any, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

// AddDetourForSubscriptions sets detour field for all nodes that belong to given subscriptions.
// A node is considered belonging to a subscription if its tag contains "[subName]" prefix.
func (u *Updater) AddDetourForSubscriptions(configPath string, subscriptionNames []string, detourValue string) error {
	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("%w %s: %v", ErrConfigFileRead, configPath, err)
	}

	// 解析JSON配置
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("%w %s: %v", ErrConfigFileParse, configPath, err)
	}

	// 检查outbounds字段
	outboundsRaw, ok := config["outbounds"]
	if !ok {
		return fmt.Errorf("%w in file %s", ErrMissingOutbounds, configPath)
	}

	outboundsArray, ok := outboundsRaw.([]any)
	if !ok {
		return fmt.Errorf("%w in file %s", ErrInvalidOutboundsFormat, configPath)
	}

	// 遍历并为匹配的节点设置 detour
	for i, outboundRaw := range outboundsArray {
		outboundMap, ok := outboundRaw.(map[string]any)
		if !ok {
			continue
		}

		tag, ok := outboundMap["tag"].(string)
		if !ok {
			continue
		}

		// 判断是否属于任一订阅
		isFromTargetSubscription := false
		for _, subName := range subscriptionNames {
			if strings.Contains(tag, fmt.Sprintf("[%s]", subName)) {
				isFromTargetSubscription = true
				break
			}
		}
		if !isFromTargetSubscription {
			continue
		}

		// 设置 detour
		outboundMap["detour"] = detourValue
		outboundsArray[i] = outboundMap
	}

	// 更新配置
	config["outbounds"] = outboundsArray

	// 写回文件
	if err := u.writeConfigFile(configPath, config); err != nil {
		return err
	}

	logger.Debug("为 %s 添加 detour 到匹配的订阅节点", configPath)
	return nil
}

// ExpandRelayNodesByDetours finds nodes that belong to given subscriptions and
// replaces each such node with multiple copies, one per detour tag provided.
// For each generated node, the "detour" field is set to the detour tag, and
// the tag is made unique by appending " -> {detourTag}".
// Returns the list of generated nodes for caching.
func (u *Updater) ExpandRelayNodesByDetours(configPath string, subscriptionNames []string, detourTags []string) ([]map[string]any, error) {
	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("%w %s: %v", ErrConfigFileRead, configPath, err)
	}

	// 解析JSON配置
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("%w %s: %v", ErrConfigFileParse, configPath, err)
	}

	// 检查outbounds字段
	outboundsRaw, ok := config["outbounds"]
	if !ok {
		return nil, fmt.Errorf("%w in file %s", ErrMissingOutbounds, configPath)
	}

	outboundsArray, ok := outboundsRaw.([]any)
	if !ok {
		return nil, fmt.Errorf("%w in file %s", ErrInvalidOutboundsFormat, configPath)
	}

	// 预处理订阅匹配函数
	belongsToTargetSubs := func(tag string) bool {
		for _, subName := range subscriptionNames {
			if strings.Contains(tag, fmt.Sprintf("[%s]", subName)) {
				return true
			}
		}
		return false
	}

	var newOutbounds []any
	var generated []map[string]any

	for _, outboundRaw := range outboundsArray {
		outboundMap, ok := outboundRaw.(map[string]any)
		if !ok {
			// 保留非对象类型
			newOutbounds = append(newOutbounds, outboundRaw)
			continue
		}

		tag, _ := outboundMap["tag"].(string)
		if tag == "" || !belongsToTargetSubs(tag) {
			// 非目标订阅，原样保留
			newOutbounds = append(newOutbounds, outboundMap)
			continue
		}

		// 目标订阅节点：为每个 detour tag 生成一个新节点
		for _, detour := range detourTags {
			if detour == "" {
				continue
			}
			nm := cloneMap(outboundMap)
			nm["detour"] = detour
			// 生成唯一标签，避免与现有重复
			if baseTag, ok := nm["tag"].(string); ok && baseTag != "" {
				nm["tag"] = fmt.Sprintf("%s -> %s", baseTag, detour)
			}
			newOutbounds = append(newOutbounds, nm)
			generated = append(generated, nm)
		}
		// 原始目标订阅节点不再保留（已被展开替代）
	}

	// 更新配置
	config["outbounds"] = newOutbounds

	// 写回文件
	if err := u.writeConfigFile(configPath, config); err != nil {
		return nil, err
	}

	logger.Debug("在 %s 为 %d 个订阅节点按 %d 个 detour 展开", configPath, len(generated), len(detourTags))
	return generated, nil
}

// InsertRealNodes inserts real proxy nodes into the configuration file without updating selectors.
// This method only handles the insertion of actual proxy nodes into the outbounds array.
func (u *Updater) InsertRealNodes(configPath string, nodes []map[string]any, subscriptionNames []string) error {
	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("%w %s: %v", ErrConfigFileRead, configPath, err)
	}

	// 解析JSON配置
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("%w %s: %v", ErrConfigFileParse, configPath, err)
	}

	// 检查outbounds字段
	outboundsRaw, ok := config["outbounds"]
	if !ok {
		return fmt.Errorf("%w in file %s", ErrMissingOutbounds, configPath)
	}

	outboundsArray, ok := outboundsRaw.([]any)
	if !ok {
		return fmt.Errorf("%w in file %s", ErrInvalidOutboundsFormat, configPath)
	}

	// 清理旧的订阅节点
	newOutbounds := u.removeOldSubscriptionNodes(outboundsArray, subscriptionNames)

	// 将真实节点插入配置中
	for _, node := range nodes {
		newOutbounds = append(newOutbounds, node)
	}

	// 更新配置
	config["outbounds"] = newOutbounds

	// 写回文件
	if err := u.writeConfigFile(configPath, config); err != nil {
		return err
	}

	logger.Debug("插入节点: %s (%d个)", configPath, len(nodes))
	return nil
}

// UpdateSelectorOnly updates only the selector outbounds list without inserting real nodes.
// This method only handles updating the selector's outbounds array based on filtering rules.
func (u *Updater) UpdateSelectorOnly(configPath string, nodes []map[string]any, subscriptionNames []string, includeKeywords []string, excludeKeywords []string) error {
	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("%w %s: %v", ErrConfigFileRead, configPath, err)
	}

	// 解析JSON配置
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("%w %s: %v", ErrConfigFileParse, configPath, err)
	}

	// 检查outbounds字段
	outboundsRaw, ok := config["outbounds"]
	if !ok {
		return fmt.Errorf("%w in file %s", ErrMissingOutbounds, configPath)
	}

	outboundsArray, ok := outboundsRaw.([]any)
	if !ok {
		return fmt.Errorf("%w in file %s", ErrInvalidOutboundsFormat, configPath)
	}

	// 查找插入标记
	_, markerOutbound, err := u.findInsertMarker(outboundsArray)
	if err != nil {
		return err
	}

	// 验证插入标记类型
	if err := u.validateMarkerType(markerOutbound); err != nil {
		return err
	}

	// 根据proxies里指定的规则更新selector
	if err := u.updateSelectorOutbounds(outboundsArray, nodes, subscriptionNames, includeKeywords, excludeKeywords); err != nil {
		return err
	}

	// 更新配置
	config["outbounds"] = outboundsArray

	// 写回文件
	if err := u.writeConfigFile(configPath, config); err != nil {
		return err
	}

	return nil
}

// UpdateConfigFile updates the specified configuration file by inserting nodes at the marker position.
// Parameters:
//   - configPath: path to the configuration file to update
//   - nodes: list of proxy nodes to insert (real nodes; not filtered by per-rule include/exclude)
//   - subscriptionNames: list of subscription names used to identify and clean old subscription nodes
//   - includeKeywords: only affect selector tag insertion (if non-empty, only tags containing any will be added)
//   - excludeKeywords: only affect selector tag insertion (tags containing any will be removed)
func (u *Updater) UpdateConfigFile(configPath string, nodes []map[string]any, subscriptionNames []string, includeKeywords []string, excludeKeywords []string) error {
	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		logger.Error("%v %s: %v", ErrConfigFileRead, configPath, err)
		return fmt.Errorf("%w %s: %v", ErrConfigFileRead, configPath, err)
	}

	// 解析JSON配置
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		logger.Error("%v %s: %v", ErrConfigFileParse, configPath, err)
		return fmt.Errorf("%w %s: %v", ErrConfigFileParse, configPath, err)
	}

	// 检查outbounds字段
	outboundsRaw, ok := config["outbounds"]
	if !ok {
		logger.Error("%v: %s", ErrMissingOutbounds, configPath)
		return fmt.Errorf("%w in file %s", ErrMissingOutbounds, configPath)
	}

	outboundsArray, ok := outboundsRaw.([]any)
	if !ok {
		logger.Error("%v: %s", ErrInvalidOutboundsFormat, configPath)
		return fmt.Errorf("%w in file %s", ErrInvalidOutboundsFormat, configPath)
	}

	// 查找插入标记
	_, markerOutbound, err := u.findInsertMarker(outboundsArray)
	if err != nil {
		logger.Error("查找插入标记失败 %s: %v", configPath, err)
		return err
	}

	// 验证插入标记类型
	if err := u.validateMarkerType(markerOutbound); err != nil {
		logger.Error("插入标记验证失败 %s: %v", configPath, err)
		return err
	}

	// 清理旧的订阅节点
	newOutbounds := u.removeOldSubscriptionNodes(outboundsArray, subscriptionNames)

	// 将真实节点插入配置中
	logger.Debug("将 %d 个真实节点插入到 outbounds", len(nodes))
	for _, node := range nodes {
		newOutbounds = append(newOutbounds, node)
	}

	// 根据proxies里指定的规则更新selector
	logger.Debug("根据proxies规则更新selector '%s' (include=%v, exclude=%v)", u.insertMarker, includeKeywords, excludeKeywords)
	if err := u.updateSelectorOutbounds(newOutbounds, nodes, subscriptionNames, includeKeywords, excludeKeywords); err != nil {
		logger.Error("更新selector outbounds失败 %s: %v", configPath, err)
		return err
	}

	// 更新配置
	config["outbounds"] = newOutbounds

	// 写回文件
	if err := u.writeConfigFile(configPath, config); err != nil {
		logger.Error("写入配置文件失败 %s: %v", configPath, err)
		return err
	}

	logger.Debug("成功更新配置文件: %s", configPath)
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
func (u *Updater) updateSelectorOutbounds(outbounds []any, nodes []map[string]any, subscriptionNames []string, includeKeywords []string, excludeKeywords []string) error {
	// 收集新节点的标签
	var nodeTags []string
	for _, node := range nodes {
		if tag, ok := node["tag"].(string); ok {
			nodeTags = append(nodeTags, tag)
		}
	}

	// 对将要添加到 selector 的标签应用 include/exclude 过滤
	filterForSelector := func(tags []string) []string {
		var included []string
		if len(includeKeywords) > 0 {
			for _, t := range tags {
				for _, kw := range includeKeywords {
					if kw == "" {
						continue
					}
					if strings.Contains(t, kw) {
						included = append(included, t)
						break
					}
				}
			}
		} else {
			included = append(included, tags...)
		}

		if len(excludeKeywords) == 0 {
			return included
		}

		var result []string
		for _, t := range included {
			skip := false
			for _, kw := range excludeKeywords {
				if kw == "" {
					continue
				}
				if strings.Contains(t, kw) {
					skip = true
					break
				}
			}
			if !skip {
				result = append(result, t)
			}
		}
		return result
	}

	beforeFilterCount := len(nodeTags)
	nodeTags = filterForSelector(nodeTags)

	// 找到并更新插入标记的outbounds列表
	for i, outboundRaw := range outbounds {
		if outboundMap, ok := outboundRaw.(map[string]any); ok {
			if tag, ok := outboundMap["tag"].(string); ok && tag == u.insertMarker {
				// 更新selector的outbounds列表
				if outboundList, ok := outboundMap["outbounds"].([]any); ok {
					// 先移除当前订阅（subscriptionNames）对应的旧标签；
					// 同时将保留项分成两类：
					// 1) baseTags：非订阅类（如固定项 direct/urltest、自定义固定字符串、非字符串等）
					// 2) otherSubTags：其它订阅来源的标签（包含形如 "[xxx]" 的字符串）
					var baseTags []any
					var otherSubTags []any
					for _, ob := range outboundList {
						s, isString := ob.(string)
						if isString {
							// 跳过需要被清理的旧订阅标签
							shouldRemove := false
							for _, subName := range subscriptionNames {
								if strings.Contains(s, fmt.Sprintf("[%s]", subName)) {
									shouldRemove = true
									break
								}
							}
							if shouldRemove {
								continue
							}

							// 粗略判断是否为订阅标签：包含方括号视为订阅来源
							if strings.Contains(s, "[") && strings.Contains(s, "]") {
								otherSubTags = append(otherSubTags, s)
							} else {
								baseTags = append(baseTags, s)
							}
						} else {
							// 保留非字符串项到基类
							baseTags = append(baseTags, ob)
						}
					}

					// 期望的顺序：基类固定项 -> 本次新增（nodeTags）-> 其它订阅标签
					var newOutboundList []any
					newOutboundList = append(newOutboundList, baseTags...)
					for _, t := range nodeTags {
						newOutboundList = append(newOutboundList, t)
					}
					newOutboundList = append(newOutboundList, otherSubTags...)

					outboundMap["outbounds"] = newOutboundList
					logger.Debug("更新selector %s: %d -> %d 标签", u.insertMarker, beforeFilterCount, len(nodeTags))
				} else {
					// 如果outbounds字段不存在，直接设置为节点标签数组
					var newOutboundList []any
					for _, tag := range nodeTags {
						newOutboundList = append(newOutboundList, tag)
					}
					outboundMap["outbounds"] = newOutboundList
					logger.Debug("创建selector %s: %d 标签", u.insertMarker, len(nodeTags))
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
