// Package security 提供安全相关的 HTTP 中间件。
//
// 该文件实现 HTTP Basic 认证中间件，支持安全的密码哈希
// （bcrypt 和 argon2id）。默认强制使用 HTTPS。
//
// 使用示例：
//
//	cfg := &config.AuthConfig{
//	    Type:       "basic",
//	    RequireTLS: true,
//	    Algorithm:  "bcrypt",
//	    Users: []config.User{
//	        {Name: "admin", Password: "$2b$12$..."}, // bcrypt 哈希
//	    },
//	    Realm: "Restricted Area",
//	}
//
//	auth, err := security.NewBasicAuth(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 应用为中间件
//	chain := middleware.NewChain(auth)
//	handler := chain.Apply(finalHandler)
//
// 作者：xfy
package security

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/valyala/fasthttp"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"

	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/middleware"
)

// HashAlgorithm 表示密码哈希算法类型。
type HashAlgorithm int

const (
	// HashBcrypt bcrypt 算法（默认，推荐）
	HashBcrypt HashAlgorithm = iota
	// HashArgon2id Argon2id 算法（更安全，计算密集）
	HashArgon2id
)

// BasicAuth 实现 HTTP Basic 认证中间件。
type BasicAuth struct {
	users        map[string]string
	realm        string
	algorithm    HashAlgorithm
	mu           sync.RWMutex
	argon2Params argon2Params
	requireTLS   bool
}

// argon2Params 保存 Argon2id 配置参数。
type argon2Params struct {
	// time 迭代次数
	time uint32
	// memory 内存成本（KB）
	memory uint32
	// threads 并行度
	threads uint8
	// saltLen 盐长度
	saltLen uint32
	// keyLen 输出密钥长度
	keyLen uint32
}

// defaultArgon2Params 默认 Argon2id 参数（OWASP 推荐）
var defaultArgon2Params = argon2Params{
	time:    3,
	memory:  64 * 1024, // 64 MB
	threads: 4,
	saltLen: 16,
	keyLen:  32,
}

// NewBasicAuth 创建 Basic 认证中间件。
//
// 根据配置创建认证中间件实例，解析用户列表并设置哈希算法。
//
// 参数：
//   - cfg: 认证配置，包含用户列表和设置
//
// 返回值：
//   - *BasicAuth: 配置好的认证中间件
//   - error: 配置无效时返回错误
func NewBasicAuth(cfg *config.AuthConfig) (*BasicAuth, error) {
	if cfg == nil {
		return nil, errors.New("auth config is nil")
	}

	if cfg.Type != "basic" {
		return nil, fmt.Errorf("unsupported auth type: %s", cfg.Type)
	}

	if len(cfg.Users) == 0 {
		return nil, errors.New("no users configured")
	}

	auth := &BasicAuth{
		users:        make(map[string]string),
		requireTLS:   cfg.RequireTLS, // Default is true from config defaults
		argon2Params: defaultArgon2Params,
	}

	// 设置认证域
	if cfg.Realm != "" {
		auth.realm = cfg.Realm
	} else {
		auth.realm = "Restricted Area"
	}

	// 设置哈希算法
	switch strings.ToLower(cfg.Algorithm) {
	case "bcrypt", "":
		auth.algorithm = HashBcrypt
	case "argon2id":
		auth.algorithm = HashArgon2id
	default:
		return nil, fmt.Errorf("unsupported hash algorithm: %s", cfg.Algorithm)
	}

	// 加载用户
	for _, user := range cfg.Users {
		if user.Name == "" {
			return nil, errors.New("username cannot be empty")
		}
		if user.Password == "" {
			return nil, fmt.Errorf("password for user %s cannot be empty", user.Name)
		}

		// 验证密码哈希格式
		if err := validatePasswordHash(user.Password, auth.algorithm); err != nil {
			return nil, fmt.Errorf("invalid password hash for user %s: %w", user.Name, err)
		}

		auth.users[user.Name] = user.Password
	}

	return auth, nil
}

// Name 返回中间件名称。
//
// 返回值：
//   - string: 中间件标识名 "basic_auth"
func (ba *BasicAuth) Name() string {
	return "basic_auth"
}

// Process 用认证逻辑包装下一个处理器。
//
// 认证失败返回 401 Unauthorized。
//
// 参数：
//   - next: 下一个请求处理器
//
// 返回值：
//   - fasthttp.RequestHandler: 包装后的处理器
func (ba *BasicAuth) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		// 检查 TLS 要求
		if ba.requireTLS && !ctx.IsTLS() {
			ctx.Error("Forbidden: HTTPS required for authentication", fasthttp.StatusForbidden)
			return
		}

		// 提取并验证凭据
		username, password, ok := ba.extractCredentials(ctx)
		if !ok {
			ba.sendAuthChallenge(ctx)
			return
		}

		// 执行认证
		if !ba.Authenticate(username, password) {
			ba.sendAuthChallenge(ctx)
			return
		}

		// 认证成功，存储用户名到上下文（用于访问日志 $remote_user）
		ctx.SetUserValue("remote_user", username)

		// 继续执行下一个处理器
		next(ctx)
	}
}

// Authenticate 验证用户名和密码凭据。
//
// 根据配置的哈希算法验证密码，返回验证结果。
//
// 参数：
//   - username: 用户名
//   - password: 明文密码
//
// 返回值：
//   - bool: true 表示认证成功，false 表示失败
func (ba *BasicAuth) Authenticate(username, password string) bool {
	ba.mu.RLock()
	hashedPassword, exists := ba.users[username]
	ba.mu.RUnlock()

	if !exists {
		return false
	}

	switch ba.algorithm {
	case HashBcrypt:
		return authenticateBcrypt(password, hashedPassword)
	case HashArgon2id:
		return authenticateArgon2id(password, hashedPassword)
	default:
		return false
	}
}

// authenticateBcrypt 使用 bcrypt 验证密码。
//
// 参数：
//   - password: 明文密码
//   - hash: bcrypt 哈希值
//
// 返回值：
//   - bool: true 表示验证通过
func authenticateBcrypt(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// authenticateArgon2id 使用 Argon2id 验证密码。
//
// 哈希格式：$argon2id$v=19$m=<memory>,t=<time>,p=<threads>$<salt>$<hash>
//
// 参数：
//   - password: 明文密码
//   - hash: Argon2id 哈希值
//
// 返回值：
//   - bool: true 表示验证通过
func authenticateArgon2id(password, hash string) bool {
	// 解析哈希字符串
	params, salt, expectedHash, err := parseArgon2idHash(hash)
	if err != nil {
		return false
	}

	// 使用相同参数生成哈希
	actualHash := argon2.IDKey([]byte(password), salt,
		params.time, params.memory, params.threads, params.keyLen)

	// 常量时间比较
	if len(actualHash) != len(expectedHash) {
		return false
	}

	for i := range actualHash {
		if actualHash[i] != expectedHash[i] {
			return false
		}
	}

	return true
}

// parseArgon2idHash 解析 Argon2id 哈希字符串。
//
// 解析格式为 $argon2id$v=19$m=<memory>,t=<time>,p=<threads>$<salt>$<hash> 的字符串。
//
// 参数：
//   - hash: Argon2id 哈希字符串
//
// 返回值：
//   - argon2Params: 解析出的参数
//   - []byte: 盐值
//   - []byte: 哈希值
//   - error: 解析失败时返回错误
func parseArgon2idHash(hash string) (argon2Params, []byte, []byte, error) {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		return argon2Params{}, nil, nil, errors.New("invalid hash format")
	}

	if parts[1] != "argon2id" {
		return argon2Params{}, nil, nil, errors.New("not argon2id hash")
	}

	if parts[2] != "v=19" {
		return argon2Params{}, nil, nil, errors.New("unsupported argon2 version")
	}

	// 解析参数：m=<memory>,t=<time>,p=<threads>
	paramsStr := parts[3]
	params := defaultArgon2Params

	paramParts := strings.Split(paramsStr, ",")
	for _, p := range paramParts {
		kv := strings.Split(p, "=")
		if len(kv) != 2 {
			continue
		}

		switch kv[0] {
		case "m":
			params.memory = parseUint32(kv[1])
		case "t":
			params.time = parseUint32(kv[1])
		case "p":
			params.threads = parseUint8(kv[1])
		}
	}

	// 解码盐值
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return argon2Params{}, nil, nil, fmt.Errorf("invalid salt: %w", err)
	}

	// 解码哈希
	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return argon2Params{}, nil, nil, fmt.Errorf("invalid hash: %w", err)
	}

	params.keyLen = uint32(len(expectedHash))

	return params, salt, expectedHash, nil
}

// extractCredentials 从 Authorization 头部提取用户名和密码。
//
// 解析 Basic 认证头部的 Base64 编码凭据。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//
// 返回值：
//   - string: 用户名
//   - string: 密码
//   - bool: 提取成功返回 true
func (ba *BasicAuth) extractCredentials(ctx *fasthttp.RequestCtx) (string, string, bool) {
	authHeader := ctx.Request.Header.Peek("Authorization")
	if len(authHeader) == 0 {
		return "", "", false
	}

	// 检查 "Basic" 前缀
	authStr := string(authHeader)
	if !strings.HasPrefix(authStr, "Basic ") {
		return "", "", false
	}

	// 解码 Base64 凭据
	encoded := strings.TrimPrefix(authStr, "Basic ")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", false
	}

	// 分割用户名:密码
	credentials := string(decoded)
	idx := strings.Index(credentials, ":")
	if idx == -1 {
		return "", "", false
	}

	username := credentials[:idx]
	password := credentials[idx+1:]

	return username, password, true
}

// sendAuthChallenge 发送 401 Unauthorized 和 Basic Auth 质询。
//
// 设置 WWW-Authenticate 响应头，要求客户端提供凭据。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
func (ba *BasicAuth) sendAuthChallenge(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("WWW-Authenticate",
		fmt.Sprintf("Basic realm=\"%s\", charset=\"UTF-8\"", ba.realm))
	ctx.Error("Unauthorized", fasthttp.StatusUnauthorized)
}

// AddUser 动态添加新用户。
//
// 密码应预先哈希。使用写锁保护并发访问。
//
// 参数：
//   - username: 用户名
//   - hashedPassword: 已哈希的密码
//
// 返回值：
//   - error: 用户名为空或密码哈希格式无效时返回错误
func (ba *BasicAuth) AddUser(username, hashedPassword string) error {
	ba.mu.Lock()
	defer ba.mu.Unlock()

	if username == "" {
		return errors.New("username cannot be empty")
	}

	if err := validatePasswordHash(hashedPassword, ba.algorithm); err != nil {
		return fmt.Errorf("invalid password hash: %w", err)
	}

	ba.users[username] = hashedPassword
	return nil
}

// RemoveUser 删除用户。
//
// 参数：
//   - username: 要删除的用户名
func (ba *BasicAuth) RemoveUser(username string) {
	ba.mu.Lock()
	delete(ba.users, username)
	ba.mu.Unlock()
}

// UpdateUser 更新用户的密码哈希。
//
// 参数：
//   - username: 用户名
//   - hashedPassword: 新的已哈希密码
//
// 返回值：
//   - error: 更新失败时返回错误
func (ba *BasicAuth) UpdateUser(username, hashedPassword string) error {
	return ba.AddUser(username, hashedPassword)
}

// HasUser 检查用户是否存在。
//
// 参数：
//   - username: 用户名
//
// 返回值：
//   - bool: 用户存在返回 true
func (ba *BasicAuth) HasUser(username string) bool {
	ba.mu.RLock()
	defer ba.mu.RUnlock()
	return ba.users[username] != ""
}

// UserCount 返回已配置用户的数量。
//
// 返回值：
//   - int: 用户数量
func (ba *BasicAuth) UserCount() int {
	ba.mu.RLock()
	defer ba.mu.RUnlock()
	return len(ba.users)
}

// validatePasswordHash 验证密码哈希格式。
//
// 根据算法类型检查哈希字符串的前缀格式。
//
// 参数：
//   - hash: 密码哈希字符串
//   - algorithm: 哈希算法类型
//
// 返回值：
//   - error: 格式无效时返回错误
func validatePasswordHash(hash string, algorithm HashAlgorithm) error {
	switch algorithm {
	case HashBcrypt:
		// bcrypt 哈希格式：$2b$<cost>$<salt><hash>
		if !strings.HasPrefix(hash, "$2") {
			return errors.New("invalid bcrypt hash format")
		}
		return nil
	case HashArgon2id:
		// argon2id 哈希格式：$argon2id$v=19$m=...,t=...,p=...$<salt>$<hash>
		if !strings.HasPrefix(hash, "$argon2id$") {
			return errors.New("invalid argon2id hash format")
		}
		return nil
	default:
		return errors.New("unknown algorithm")
	}
}

// HashPassword 使用配置的算法生成密码哈希。
//
// 这是用于生成配置文件中使用的哈希的工具函数。
//
// 参数：
//   - password: 明文密码
//   - algorithm: 哈希算法
//
// 返回值：
//   - string: 生成的哈希字符串
//   - error: 生成失败时返回错误
func HashPassword(password string, algorithm HashAlgorithm) (string, error) {
	switch algorithm {
	case HashBcrypt:
		return HashPasswordBcrypt(password, bcrypt.DefaultCost)
	case HashArgon2id:
		return HashPasswordArgon2id(password, defaultArgon2Params)
	default:
		return "", errors.New("unknown algorithm")
	}
}

// HashPasswordBcrypt 生成 bcrypt 哈希。
//
// 参数：
//   - password: 明文密码
//   - cost: 计算成本（推荐 bcrypt.DefaultCost）
//
// 返回值：
//   - string: bcrypt 哈希字符串
//   - error: 生成失败时返回错误
func HashPasswordBcrypt(password string, cost int) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// HashPasswordArgon2id 生成 Argon2id 哈希。
//
// 使用指定的参数生成安全的密码哈希。
//
// 参数：
//   - password: 明文密码
//   - params: Argon2id 配置参数
//
// 返回值：
//   - string: Argon2id 哈希字符串
//   - error: 生成失败时返回错误
func HashPasswordArgon2id(password string, params argon2Params) (string, error) {
	// 使用加密安全的随机数生成盐值
	salt := make([]byte, params.saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	// 生成哈希
	hash := argon2.IDKey([]byte(password), salt,
		params.time, params.memory, params.threads, params.keyLen)

	// 编码为字符串格式
	encodedSalt := base64.RawStdEncoding.EncodeToString(salt)
	encodedHash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		params.memory, params.time, params.threads, encodedSalt, encodedHash), nil
}

// parseUint32 将字符串解析为 uint32。
//
// 参数：
//   - s: 数字字符串
//
// 返回值：
//   - uint32: 解析结果
func parseUint32(s string) uint32 {
	var result uint32
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + uint32(c-'0')
		}
	}
	return result
}

// parseUint8 将字符串解析为 uint8。
//
// 参数：
//   - s: 数字字符串
//
// 返回值：
//   - uint8: 解析结果
func parseUint8(s string) uint8 {
	var result uint8
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + uint8(c-'0')
		}
	}
	return result
}

// 验证接口实现
var _ middleware.Middleware = (*BasicAuth)(nil)
