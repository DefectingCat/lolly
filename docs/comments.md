# Golang 注释规范

本文档定义了 Cadmus 项目中 Go 代码的注释标准，所有开发者应遵循这些规范以确保代码的可读性和可维护性。

## 1. 文件头注释

每个 Go 源文件必须在开头添加文件头注释，说明文件的作用、主要功能和所属模块。

### 格式

```go
// Package xxx 提供了 xxx 功能的实现。
//
// 该文件包含 xxx 相关的核心逻辑，包括：
//   - 功能 A 的实现
//   - 功能 B 的处理
//   - 与 xxx 服务的交互
//
// 主要用途：
//   用于处理 xxx 场景下的 xxx 业务逻辑。
//
// 注意事项：
//   - 使用前需要确保 xxx 条件满足
//   - 并发安全/非并发安全
//
// 作者：xfy
package xxx
```

### 示例

```go
// Package services 提供了 Cadmus 核心业务服务的实现。
//
// 该文件包含媒体服务相关的核心逻辑，包括：
//   - 媒体文件的存储和管理
//   - 媒体元数据的解析和处理
//   - 与存储后端的交互
//
// 主要用途：
//   用于处理用户上传媒体文件的完整生命周期管理。
//
// 注意事项：
//   - 所有公开方法均为并发安全
//   - 使用前需确保存储服务已正确初始化
//
// 作者：xfy
package services
```

## 2. 函数注释

每个函数（包括公开函数和私有函数）都必须添加注释，说明其用途、参数、返回值和使用注意事项。

### 格式

```go
// FunctionName 执行 xxx 操作。
//
// 该函数用于处理 xxx 场景，实现 xxx 功能。
//
// 参数：
//   - param1: 参数1的说明，类型为 xxx，用于 xxx
//   - param2: 参数2的说明，类型为 xxx，用于 xxx
//
// 返回值：
//   - result: 返回结果的说明，类型为 xxx
//   - err: 错误信息，当 xxx 时返回非 nil 值
//
// 使用示例：
//   result, err := FunctionName(param1, param2)
//   if err != nil {
//       // 处理错误
//   }
//
// 注意事项：
//   - 调用前需确保 xxx 条件满足
//   - 该函数不处理 xxx 情况，需要调用方自行处理
func FunctionName(param1 Type1, param2 Type2) (Result, error) {
    // 实现
}
```

### 简单函数格式

对于功能单一、逻辑简单的函数，可使用简化格式：

```go
// ValidateInput 验证用户输入是否有效。
//
// 检查输入字符串是否非空且符合长度要求（1-100字符）。
// 返回 true 表示验证通过，false 表示验证失败。
func ValidateInput(input string) bool {
    return len(input) > 0 && len(input) <= 100
}
```

### 公开函数示例

```go
// SaveMedia 保存媒体文件到存储后端。
//
// 该方法将媒体文件及其元数据持久化到配置的存储系统中。
// 支持多种存储后端，包括本地文件系统、S3 和 MinIO。
//
// 参数：
//   - ctx: 上下文，用于控制超时和取消操作
//   - media: 媒体对象，包含文件数据和元数据
//
// 返回值：
//   - id: 保存成功后生成的唯一标识符
//   - err: 可能的错误包括：
//       - ErrStorageUnavailable: 存储服务不可用
//       - ErrInvalidMedia: 媒体数据无效
//       - ErrQuotaExceeded: 存储配额超限
//
// 使用示例：
//   id, err := service.SaveMedia(ctx, media)
//   if errors.Is(err, services.ErrStorageUnavailable) {
//       // 重试或切换存储后端
//   }
//
// 注意事项：
//   - 该方法是并发安全的，可在多个 goroutine 中同时调用
//   - 大文件上传建议使用 StreamMedia 方法
func (s *MediaService) SaveMedia(ctx context.Context, media *Media) (string, error) {
    // 实现
}
```

### 私有函数示例

```go
// generateUniqueID 生成唯一的媒体标识符。
//
// 使用时间戳和随机数组合生成 UUID 格式的标识符。
// 保证在分布式环境下的唯一性。
//
// 返回值：
//   - 返回格式为 "media-{timestamp}-{random}" 的唯一字符串
func generateUniqueID() string {
    // 实现
}
```

## 3. 代码步骤注释

代码中的关键步骤、复杂逻辑、重要决策点都需要添加注释说明。

### 块注释格式

```go
func ProcessData(data []byte) error {
    // 步骤1: 验证数据格式
    // 检查数据是否符合预期的 JSON 格式，并包含必要字段
    if err := validateFormat(data); err != nil {
        return fmt.Errorf("数据格式验证失败: %w", err)
    }

    // 步骤2: 解析数据内容
    // 将 JSON 字节数组转换为结构体，便于后续处理
    var payload DataPayload
    if err := json.Unmarshal(data, &payload); err != nil {
        return fmt.Errorf("数据解析失败: %w", err)
    }

    // 步骤3: 数据转换
    // 将原始数据转换为业务模型，应用必要的转换规则
    model := transformToModel(payload)

    // 步骤4: 持久化存储
    // 将处理后的数据保存到数据库，使用事务确保数据一致性
    if err := saveWithTransaction(model); err != nil {
        return fmt.Errorf("数据保存失败: %w", err)
    }

    return nil
}
```

### 行内注释格式

对于单行代码的解释，使用行末注释：

```go
func CalculateScore(items []Item) int {
    total := 0
    for _, item := range items {
        // 权重计算：基础分数乘以优先级系数
        weightedScore := item.BaseScore * item.PriorityFactor
        total += weightedScore
    }

    // 应用上限约束，防止分数溢出
    if total > MaxScore {
        total = MaxScore
    }

    return total
}
```

### 复杂逻辑注释示例

```go
func (s *Service) ProcessRequest(ctx context.Context, req *Request) (*Response, error) {
    // === 预处理阶段 ===

    // 验证请求参数完整性
    // 必须字段包括：ID、Type、Timestamp
    if err := s.validateRequest(req); err != nil {
        return nil, err
    }

    // 检查请求是否已处理过
    // 通过缓存查询避免重复处理，提高系统效率
    if cached, exists := s.cache.Get(req.ID); exists {
        // 缓存命中，直接返回缓存结果
        return cached.(*Response), nil
    }

    // === 核心处理阶段 ===

    // 根据请求类型选择处理策略
    // 不同类型有不同的处理逻辑和性能特征
    handler := s.getHandler(req.Type)
    if handler == nil {
        // 未找到处理器，返回不支持错误
        return nil, ErrUnsupportedType
    }

    // 执行处理逻辑
    // 注意：此处可能耗时较长，已通过 ctx 控制超时
    result, err := handler.Handle(ctx, req)
    if err != nil {
        // 处理失败，记录详细错误信息用于排查
        s.logger.Error("处理失败", "request_id", req.ID, "error", err)
        return nil, err
    }

    // === 后处理阶段 ===

    // 构建响应对象
    response := &Response{
        ID:      req.ID,
        Result:  result,
        Status:  StatusSuccess,
        Created: time.Now(),
    }

    // 缓存响应结果，有效期 5 分钟
    // 热点请求的缓存可显著降低系统负载
    s.cache.Set(req.ID, response, 5*time.Minute)

    return response, nil
}
```

### 特殊情况注释

```go
func LoadConfig(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        // 文件不存在时返回默认配置，而非错误
        // 设计决策：允许用户不提供配置文件，使用系统默认值
        if os.IsNotExist(err) {
            return DefaultConfig(), nil
        }
        return nil, err
    }

    // 注意：此处故意忽略未知字段
    // 原因：支持配置文件向后兼容，旧版本配置文件可在新版本中使用
    var config Config
    decoder := json.NewDecoder(bytes.NewReader(data))
    decoder.DisallowUnknownFields = false // 允许未知字段
    if err := decoder.Decode(&config); err != nil {
        return nil, err
    }

    return &config, nil
}
```

## 4. 注释最佳实践

### 使用中文注释

所有注释必须使用中文，确保团队成员易于理解：

```go
// 正确 ✓
// 获取用户信息，根据用户 ID 查询数据库返回完整用户对象。

// 错误 ✗
// Get user info by ID from database.
```

### 注释内容要求

注释应说明"为什么"而非仅仅是"是什么"：

```go
// 正确 ✓ - 解释原因
// 使用批处理而非逐条处理，原因是：
// 1. 减少 I/O 操作次数，从 N 次降为 1 次
// 2. 降低事务开销，提升整体性能约 10 倍
func batchInsert(items []Item) error {
    // ...
}

// 错误 ✗ - 仅重复代码含义
// 循环遍历所有元素
for _, item := range items {
    // ...
}
```

### 避免冗余注释

```go
// 正确 ✓ - 提供有价值的信息
// 超时时间设置为 30 秒，因为下游服务 P99 响应时间为 25 秒
const Timeout = 30 * time.Second

// 错误 ✗ - 无意义的注释
// 超时变量
const Timeout = 30 * time.Second
```

### TODO 和 FIXME 注释

使用标准格式标记待办事项：

```go
// TODO(author): 待实现功能的描述
//   - 详细说明需要做什么
//   - 预计完成时间或优先级
func unfinishedFunction() {
    // TODO(xfy): 添加缓存支持，预计 2024-04-01 完成
}

// FIXME(author): 问题描述和修复建议
func problematicFunction() {
    // FIXME(xfy): 当前实现在高并发下存在竞态条件
    // 需要添加互斥锁保护共享资源
}
```

## 5. 结构体和接口注释

### 结构体注释

```go
// Media 表示系统中的媒体文件对象。
//
// 包含媒体文件的基本信息和元数据，用于媒体管理功能。
// 所有字段在创建后不可修改（不可变设计）。
//
// 注意事项：
//   - ID 由系统自动生成，无需手动设置
//   - CreatedAt 使用 UTC 时间
type Media struct {
    // ID 媒体的唯一标识符，格式为 UUID
    ID string `json:"id"`

    // Name 原始文件名，保留用户上传时的文件名
    Name string `json:"name"`

    // Size 文件大小，单位为字节
    Size int64 `json:"size"`

    // Type 媒体类型，如 "image/jpeg"、"video/mp4"
    Type string `json:"type"`

    // CreatedAt 创建时间，使用 UTC 时间戳
    CreatedAt time.Time `json:"created_at"`

    // Metadata 扩展元数据，存储自定义属性
    Metadata map[string]string `json:"metadata,omitempty"`
}
```

### 接口注释

```go
// Storage 定义存储后端的抽象接口。
//
// 该接口抽象了不同存储系统的共性操作，支持：
//   - 本地文件系统
//   - Amazon S3
//   - MinIO
//   - 其他兼容 S3 API 的存储服务
//
// 实现要求：
//   - 所有方法必须是并发安全的
//   - 上传方法应支持大文件流式传输
//   - 错误返回应使用定义的错误类型
type Storage interface {
    // Upload 上传文件到存储后端。
    //
    // 参数：
    //   - ctx: 上下文，用于超时控制
    //   - key: 存储键名，作为文件的唯一标识
    //   - data: 文件内容字节
    //
    // 返回值：
    //   - err: 上传失败时返回错误
    Upload(ctx context.Context, key string, data []byte) error

    // Download 从存储后端下载文件。
    //
    // 参数：
    //   - ctx: 上下文，用于超时控制
    //   - key: 存储键名
    //
    // 返回值：
    //   - data: 文件内容字节
    //   - err: 文件不存在或其他错误
    Download(ctx context.Context, key string) ([]byte, error)

    // Delete 从存储后端删除文件。
    Delete(ctx context.Context, key string) error
}
```

## 6. 常量和变量注释

### 常量注释

```go
// 定义系统配置常量
const (
    // DefaultPort 默认服务端口
    // 当配置文件未指定端口时使用此值
    DefaultPort = 8080

    // MaxUploadSize 最大上传文件大小限制
    // 设为 100MB，平衡存储成本和用户需求
    MaxUploadSize = 100 * 1024 * 1024

    // CacheTTL 缓存默认过期时间
    // 5 分钟可覆盖大多数热点请求场景
    CacheTTL = 5 * time.Minute
)
```

### 变量注释

```go
var (
    // ErrStorageUnavailable 存储服务不可用错误
    // 当无法连接到存储后端时返回
    ErrStorageUnavailable = errors.New("存储服务不可用")

    // ErrInvalidMedia 无效媒体错误
    // 当媒体文件格式或内容不符合要求时返回
    ErrInvalidMedia = errors.New("无效的媒体文件")

    // ErrQuotaExceeded 配额超限错误
    // 当用户存储空间达到上限时返回
    ErrQuotaExceeded = errors.New("存储配额已超限")
)
```

## 7. 包文档注释

每个包应有一个 doc.go 文件或在其主要文件中添加包级注释：

```go
// Package services 提供 Cadmus 的核心业务服务层实现。
//
// 该包包含以下主要服务：
//
//   - MediaService: 媒体文件管理服务
//   - AuthService: 用户认证和授权服务
//   - UserService: 用户信息管理服务
//   - PluginService: 插件系统管理服务
//
// 服务设计原则：
//
//   1. 所有服务通过接口定义，便于测试和替换实现
//   2. 服务间通过依赖注入解耦
//   3. 所有公开方法均为并发安全
//   4. 错误使用语义化错误类型，便于调用方处理
//
// 初始化示例：
//
//   cfg := services.Config{Storage: storageBackend}
//   mediaSvc := services.NewMediaService(cfg)
//   authSvc := services.NewAuthService(cfg)
//
// 注意事项：
//
//   - 使用前需确保依赖服务已正确初始化
//   - 建议通过 services.NewServices() 创建服务集合
//   - 所有服务支持通过配置启用/禁用
package services
```

## 8. 注释检查清单

在提交代码前，请检查以下事项：

- [ ] 文件头部是否有完整的文件注释
- [ ] 所有公开函数是否有详细注释（包括参数、返回值、示例）
- [ ] 所有私有函数是否有功能说明注释
- [ ] 关键逻辑步骤是否有注释说明
- [ ] 复杂算法是否有解释其原理的注释
- [ ] 特殊处理是否有解释其原因的注释
- [ ] 结构体及其字段是否有注释
- [ ] 接口及其方法是否有注释
- [ ] 常量和全局变量是否有注释
- [ ] 所有注释是否使用中文
- [ ] TODO/FIXME 是否标注了负责人和时间

---

_本规范适用于 Cadmus 项目所有 Go 源文件。_
