## 更新配置文件实例

ulw 深度分析配置文件，当前支持的配置和 --generate-config 是否有不一样的地方，--generate-config
需要完整且详细，包括所有字段的枚举值都要有

ulw 深度分析下有没有代码实现了，但是 --generate-config 缺失的地方

ulw 逐行分析下当前的 --generate-config，看看代码中是否都实现了

ulw 分析 lib/nginx 代码，然后更新 docs/nginx/ 里的文档

ulw 深度分析下 @docs/ 下的 nginx 文档，看看当前项目实现的怎么样了

## 单元测试

ulw 深度分析下当前测试覆盖率

/ralplan 深度分析一个完善测试的方案

ulw 分析并完善测试覆盖率，每完成一个功能点提交一次

## 注释

ulw 参考 @docs/comments.md，深度分析项目注释是否完善

## 优化

ulw 深度分析下，有没有重复的逻辑/代码，或者冗余的东西，或者没用的东西

ulw 运行 make lint，并修复

ulw 深度分析下当前项目的性能

ulw 完善性能基准测试

ulw 深度分析下代码质量

ulw 深度分析下代码架构

ulw 分析下 lib/fasthttp/ 的源码，然后看下 lolly 的用法合不合理，有没有性能可以提升的地方

## 兼容性

ulw @docs/config/ 下有些nginx的配置示例,深度分析下当前 lolly 项目,然后看看 lolly 是否支持实现这些 nginx 的效果

ulw 查看 @docs/nginx/04-nginx-proxy-loadbalancing.md 分析下 lolly 是否实现了这些功能
