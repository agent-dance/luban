WebSocket 路由这边我发现 `dependencies` 接不上，应用级和 router 级依赖只在 HTTP endpoint 生效。帮我让 WebSocket 路由也能使用这些依赖配置。应用级和 router 级 WebSocket dependencies 都补一下测试。
