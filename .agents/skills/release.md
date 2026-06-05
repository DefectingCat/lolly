# Skill: 发布新版本

发布 lolly 新版本的完整流程。

## 前置条件

- `gh` CLI 已安装且已认证（`gh auth status`）
- `make` 可用
- 工作目录干净（无未提交更改）

## 发布流程

### 1. 确定新版本号

遵循 semver 规范，基于上次 tag 递增：
- PATCH（x.y.Z）：bug 修复、小改进
- MINOR（x.Y.0）：新功能、向后兼容
- MAJOR（X.0.0）：破坏性变更

查看当前版本：
```bash
git tag --sort=-v:refname | head -5
```

### 2. 更新 FALLBACK_VERSION

编辑 `Makefile` 第 4 行，将 `FALLBACK_VERSION` 更新为新版本号（不带 `v` 前缀）：

```makefile
FALLBACK_VERSION := x.y.z
```

### 3. 更新 CHANGELOG.md

编辑 `CHANGELOG.md`，将 `[Unreleased]` 下方的内容整理为新版本条目：

```markdown
## [Unreleased]

## [x.y.z] - YYYY-MM-DD

### Added
- ...

### Changed
- ...

### Performance
- ...

### Fixed
- ...
```

保留 `[Unreleased]` 空头，按 Keep a Changelog 格式填写。参考历史条目的分类方式。

### 4. 提交更改

```bash
git add Makefile CHANGELOG.md
git commit -m "chore: release v<x.y.z>"
```

### 5. 质量检查

```bash
make check
```

这会运行 fmt → lint → test-all。确保全部通过后再继续。

### 6. 打 git tag

```bash
git tag v<x.y.z>
```

### 7. 推送

```bash
git push origin main --tags
```

### 8. 交叉编译所有平台

```bash
make build-all
```

产物在 `bin/` 目录：
- `lolly-linux-amd64`
- `lolly-darwin-amd64`
- `lolly-darwin-arm64`
- `lolly-windows-amd64.exe`
- `lolly-freebsd-amd64`
- `lolly-openbsd-amd64`

### 9. 创建 GitHub Release 并上传二进制

使用 `gh` CLI 创建 release 并上传所有二进制文件：

```bash
gh release create v<x.y.z> \
  bin/lolly-linux-amd64 \
  bin/lolly-darwin-amd64 \
  bin/lolly-darwin-arm64 \
  bin/lolly-windows-amd64.exe \
  bin/lolly-freebsd-amd64 \
  bin/lolly-openbsd-amd64 \
  --title "v<x.y.z>" \
  --notes "$(sed -n '/^## \[x\.y\.z\]/,/^## \[/p' CHANGELOG.md | head -n -1 | tail -n +2)"
```

这会自动从 CHANGELOG.md 提取对应版本的 release notes。

也可以用 `--draft` 先创建草稿，确认无误后再发布：

```bash
gh release create v<x.y.z> ... --draft
gh release edit v<x.y.z> --draft=false
```

### 10.（可选）构建并推送 Docker 镜像

```bash
make docker
make docker-push REGISTRY=<registry>
```

## 检查清单

- [ ] `Makefile` 的 `FALLBACK_VERSION` 已更新
- [ ] `CHANGELOG.md` 有新版本条目，日期正确
- [ ] `make check` 全部通过
- [ ] `git tag v<x.y.z>` 已创建
- [ ] tag 和 commit 已推送到 remote
- [ ] `make build-all` 6 个平台二进制编译成功
- [ ] `gh release create` 已发布，二进制已上传
- [ ] （可选）Docker 镜像已构建并推送

## 回滚

如果发布有问题，删除 tag 和 release：

```bash
gh release delete v<x.y.z> --yes
git push origin :refs/tags/v<x.y.z>
git tag -d v<x.y.z>
```
