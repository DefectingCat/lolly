# Lolly 项目缺少文档注释的导出标识符检测报告

## 概述

使用 AST 静态分析扫描 `/root/projects/lolly`，共发现 **180** 个导出的函数/类型/常量/变量缺少以标识符名称开头的文档注释（GoDoc）。

---

## 一、公共 API（非 internal 包）

| 文件 | 行号 | 类型 | 名称 |
|------|------|------|------|
| `gjson/gjson.go` | 35 | const | `ModuleName` |
| `gjson/gjson.go` | 38 | const | `Version` |

**说明**：`gjson` 是项目中唯一的非 internal 子包，其导出的 `ModuleName` 和 `Version` 常量缺少文档注释。

---

## 二、internal 包中的缺失（按文件分组）

### internal/benchmark/tools/tools.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 13 | const | `Size100B` |
| 14 | const | `Size1KB` |
| 15 | const | `Size10KB` |
| 16 | const | `Size100KB` |
| 17 | const | `Size1MB` |
| 36 | const | `ModeNormalResponse` |
| 37 | const | `ModeRandomResponse` |
| 38 | const | `ModeErrorResponse` |
| 39 | const | `ModeDelayedResponse` |

### internal/config/config.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 33 | const | `DefaultPprofPath` |
| 44 | const | `ServerModeSingle` |
| 46 | const | `ServerModeVHost` |
| 48 | const | `ServerModeMultiServer` |
| 50 | const | `ServerModeAuto` |

### internal/converter/nginx/converter.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 28 | method | `Warning.String` |

### internal/converter/nginx/parser.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 31 | method | `*ParseError.Error` |

### internal/e2e/testutil/constants.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 18 | const | `ContainerStartupTimeout` |
| 21 | const | `HealthCheckWaitTimeout` |
| 24 | const | `HealthCheckDetectionTime` |
| 27 | const | `CacheExpireBuffer` |
| 30 | const | `DefaultTestTimeout` |
| 33 | const | `DefaultClientTimeout` |
| 36 | const | `ConcurrentRequestTimeout` |
| 39 | const | `ShortTestTimeout` |
| 42 | const | `MediumTestTimeout` |
| 48 | const | `DefaultBackendCount` |
| 51 | const | `DefaultConcurrentRequests` |
| 54 | const | `HighConcurrentRequests` |
| 57 | const | `CacheTestMaxAge` |
| 60 | const | `CacheTestShortMaxAge` |
| 66 | const | `TLSVersion12` |
| 69 | const | `TLSVersion13` |

### internal/e2e/testutil/websocket.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 41 | func | `WithWSHeaders` |

### internal/lua/api_log.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 29 | const | `LogStderr` |
| 30 | const | `LogEmerg` |
| 31 | const | `LogAlert` |
| 32 | const | `LogCrit` |
| 33 | const | `LogErr` |
| 34 | const | `LogWarn` |
| 35 | const | `LogNotice` |
| 36 | const | `LogInfo` |
| 37 | const | `LogDebug` |
| 42 | const | `HTTPContinue` |
| 43 | const | `HTTPSwitchingProtocols` |
| 44 | const | `HTTPOK` |
| 45 | const | `HTTPCreated` |
| 46 | const | `HTTPAccepted` |
| 47 | const | `HTTPNoContent` |
| 48 | const | `HTTPPartialContent` |
| 49 | const | `HTTPMovedPermanently` |
| 50 | const | `HTTPFound` |
| 51 | const | `HTTPSeeOther` |
| 52 | const | `HTTPNotModified` |
| 53 | const | `HTTPTemporaryRedirect` |
| 54 | const | `HTTPPermanentRedirect` |
| 55 | const | `HTTPBadRequest` |
| 56 | const | `HTTPUnauthorized` |
| 57 | const | `HTTPForbidden` |
| 58 | const | `HTTPNotFound` |
| 59 | const | `HTTPMethodNotAllowed` |
| 60 | const | `HTTPRequestTimeout` |
| 61 | const | `HTTPConflict` |
| 62 | const | `HTTPGone` |
| 63 | const | `HTTPLengthRequired` |
| 64 | const | `HTTPPayloadTooLarge` |
| 65 | const | `HTTPURITooLong` |
| 66 | const | `HTTPUnsupportedMedia` |
| 67 | const | `HTTPRangeNotSatisfiable` |
| 68 | const | `HTTPTooManyRequests` |
| 69 | const | `HTTPInternalServerError` |
| 70 | const | `HTTPNotImplemented` |
| 71 | const | `HTTPBadGateway` |
| 72 | const | `HTTPServiceUnavailable` |
| 73 | const | `HTTPGatewayTimeout` |
| 74 | const | `HTTPHTTPVersionNotSupported` |

### internal/lua/api_req.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 36 | const | `APILayerDirect` |
| 40 | const | `APILayerCompatible` |
| 44 | const | `APILayerPseudoNonBlocking` |
| 58 | method | `ngxReqAPILayer.String` |

### internal/lua/cache.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 38 | const | `CacheKeyInline` |
| 41 | const | `CacheKeyFile` |

### internal/lua/coroutine.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 35 | const | `PhaseInit` |
| 37 | const | `PhaseRewrite` |
| 39 | const | `PhaseAccess` |
| 41 | const | `PhaseContent` |
| 43 | const | `PhaseLog` |
| 45 | const | `PhaseHeaderFilter` |
| 47 | const | `PhaseBodyFilter` |
| 50 | method | `Phase.String` |

### internal/lua/socket_manager.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 36 | const | `SocketStateIdle` |
| 38 | const | `SocketStateConnecting` |
| 40 | const | `SocketStateConnected` |
| 42 | const | `SocketStateSending` |
| 44 | const | `SocketStateReceiving` |
| 46 | const | `SocketStateClosing` |
| 48 | const | `SocketStateClosed` |
| 50 | const | `SocketStateError` |
| 83 | const | `OpConnect` |
| 85 | const | `OpSend` |
| 87 | const | `OpReceive` |
| 89 | const | `OpClose` |

### internal/matcher/location.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 257 | method | `*ConflictError.Error` |

### internal/matcher/matcher.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 27 | const | `LocationTypeExact` |
| 28 | const | `LocationTypePrefix` |
| 29 | const | `LocationTypePrefixPriority` |
| 30 | const | `LocationTypeRegex` |
| 31 | const | `LocationTypeRegexCaseless` |
| 32 | const | `LocationTypeNamed` |

### internal/middleware/compression/compression.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 100 | const | `AlgorithmGzip` |
| 102 | const | `AlgorithmBrotli` |

### internal/middleware/limitrate/limitrate.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 15 | const | `LargeFileStrategySkip` |
| 17 | const | `LargeFileStrategyCoarse` |

### internal/middleware/rewrite/rewrite.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 41 | const | `FlagLast` |
| 44 | const | `FlagRedirect` |
| 47 | const | `FlagPermanent` |
| 50 | const | `FlagBreak` |

### internal/middleware/security/access.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 47 | const | `ActionAllow` |
| 49 | const | `ActionDeny` |

### internal/middleware/security/auth.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 52 | const | `HashBcrypt` |
| 54 | const | `HashArgon2id` |

### internal/resolver/resolver.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 446 | method | `*noopResolver.LookupHost` |
| 450 | method | `*noopResolver.LookupHostWithCache` |
| 454 | method | `*noopResolver.Refresh` |
| 458 | method | `*noopResolver.Start` |
| 462 | method | `*noopResolver.Stop` |
| 466 | method | `*noopResolver.Stats` |

### internal/ssl/client_verify.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 33 | const | `VerifyOff` |
| 35 | const | `VerifyOn` |
| 37 | const | `VerifyOptional` |
| 39 | const | `VerifyOptionalNoCA` |

### internal/stream/stream.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 82 | method | `*roundRobin.Select` |
| 171 | method | `*weightedRoundRobin.Select` |
| 244 | method | `*ipHash.Select` |
| 248 | method | `*ipHash.SelectByIP` |

### internal/utils/httperror.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 26 | var | `ErrNotFound` |
| 27 | var | `ErrForbidden` |
| 28 | var | `ErrUnauthorized` |
| 29 | var | `ErrBadGateway` |
| 30 | var | `ErrGatewayTimeout` |
| 31 | var | `ErrInternalError` |
| 32 | var | `ErrTooManyRequests` |
| 33 | var | `ErrServiceUnavailable` |

### internal/variable/builtin.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 19 | const | `VarHost` |
| 20 | const | `VarRemoteAddr` |
| 21 | const | `VarRemotePort` |
| 22 | const | `VarRequestURI` |
| 23 | const | `VarURI` |
| 24 | const | `VarArgs` |
| 25 | const | `VarRequestMethod` |
| 26 | const | `VarScheme` |
| 27 | const | `VarServerName` |
| 28 | const | `VarServerPort` |
| 29 | const | `VarStatus` |
| 30 | const | `VarBodyBytesSent` |
| 31 | const | `VarRequestTime` |
| 32 | const | `VarTimeLocal` |
| 33 | const | `VarTimeISO8601` |
| 34 | const | `VarRequestID` |
| 36 | const | `VarUpstreamAddr` |
| 37 | const | `VarUpstreamStatus` |
| 38 | const | `VarUpstreamResponseTime` |
| 39 | const | `VarUpstreamConnectTime` |
| 40 | const | `VarUpstreamHeaderTime` |

### internal/variable/ssl.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 24 | const | `VarSSLClientVerify` |
| 25 | const | `VarSSLClientSerial` |
| 26 | const | `VarSSLClientSubject` |
| 27 | const | `VarSSLClientIssuer` |
| 28 | const | `VarSSLClientFingerprint` |
| 29 | const | `VarSSLClientNotBefore` |
| 30 | const | `VarSSLClientNotAfter` |
| 31 | const | `VarSSLClientDNS` |
| 32 | const | `VarSSLClientEmail` |

### internal/version/version.go
| 行号 | 类型 | 名称 |
|------|------|------|
| 9 | var | `Version` |
| 11 | var | `GitCommit` |
| 13 | var | `GitBranch` |
| 15 | var | `BuildTime` |
| 17 | var | `GoVersion` |
| 19 | var | `BuildPlatform` |

---

## 统计汇总

| 类别 | 数量 |
|------|------|
| 公共 API（非 internal） | 2 |
| internal 常量 | 134 |
| internal 变量 | 14 |
| internal 方法 | 14 |
| internal 函数 | 1 |
| **总计** | **180** |

---

*检测规则：导出的标识符（首字母大写）必须紧跟以该标识符名称开头的 `//` 文档注释，才被视为已文档化。`_test.go` 和 vendor 目录已排除。*
