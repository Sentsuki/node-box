# 模块缓存机制使用示例

## 修改说明

已经成功为模块管理器添加了缓存机制，仿照节点管理器的实现。现在模块获取也只会请求一次，直到缓存失效。

## 主要修改

### 1. 添加了 ModuleCache 结构体
```go
type ModuleCache struct {
    modules map[string]map[string]any // module name -> module data
    valid   bool                      // 缓存是否有效
}
```

### 2. 修改了 ModuleManager 结构体
- 移除了直接的 `modules` 字段
- 添加了 `cache *ModuleCache` 字段

### 3. 添加了缓存管理方法
- `InvalidateCache()`: 失效缓存，强制下次获取时重新请求
- 修改了 `FetchAllModules()`: 现在会检查缓存是否有效，只在需要时才重新获取

### 4. 更新了所有访问模块数据的方法
- `GetModule()`: 从缓存中获取模块
- `GetModulesByType()`: 从缓存中获取指定类型的模块
- `ListModules()`: 从缓存中列出所有模块名称
- `HasModule()`: 检查缓存中是否存在指定模块

## 缓存机制工作流程

1. **首次调用 FetchAllModules()**:
   - 检查缓存是否有效 (`cache.valid == false`)
   - 清空缓存并重新获取所有模块
   - 将获取的模块存储到缓存中
   - 标记缓存为有效 (`cache.valid = true`)

2. **后续调用 FetchAllModules()**:
   - 检查缓存是否有效 (`cache.valid == true`)
   - 直接返回，不重新请求模块数据
   - 输出日志: "使用缓存的模块数据，共 X 个模块"

3. **强制刷新**:
   - 调用 `InvalidateCache()` 失效缓存
   - 下次调用 `FetchAllModules()` 时会重新获取

## 与节点缓存的一致性

现在模块管理器的缓存机制与节点管理器完全一致：

- **节点**: `SubscriptionCache` -> `FetchAndCacheAllSubscriptions()` -> `FetchNodesFromSubscriptions()`
- **模块**: `ModuleCache` -> `FetchAllModules()` -> `GetModule()`/`GetModulesByType()`

## 在 UpdateAllConfigurations() 中的使用

```go
func (nm *NodeManager) UpdateAllConfigurations() error {
    // 1. 失效所有缓存，确保获取最新数据
    nm.InvalidateCache()           // 节点缓存
    nm.moduleManager.InvalidateCache() // 模块缓存
    
    // 2. 更新节点配置（只请求一次所有订阅）
    if err := nm.UpdateAllConfigs(); err != nil {
        // 处理错误
    }
    
    // 3. 更新模块配置（只请求一次所有模块）
    if err := nm.UpdateModuleConfigs(); err != nil {
        // 处理错误
    }
}
```

## 优势

1. **性能提升**: 避免重复的网络请求和文件读取
2. **一致性**: 与节点管理器的缓存机制保持一致
3. **可控性**: 可以通过 `InvalidateCache()` 强制刷新
4. **错误处理**: 保持了原有的错误处理逻辑
5. **日志记录**: 清楚地记录缓存使用情况

## 使用场景

- **批量更新**: 在一次更新周期中，多次调用模块相关方法时只请求一次
- **配置文件更新**: 更新多个配置文件时，模块数据只获取一次
- **开发调试**: 可以通过日志清楚地看到是使用缓存还是重新获取