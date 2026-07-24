我在接代理和重定向这块时发现 header 处理不够稳，Authorization/Cookie 这类东西可能被带到不该去的地方。麻烦把 proxy header、普通请求 header、跨站 redirect 的边界收紧。
