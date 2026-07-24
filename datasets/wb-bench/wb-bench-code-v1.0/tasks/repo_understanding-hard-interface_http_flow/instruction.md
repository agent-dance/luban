我这里要接一个 Feign 相关的改动，但现在还没把它从 Java 接口方法一路变成 HTTP 请求、再把响应解回返回值的链路看明白。

请你不要改业务代码，只在仓库根目录生成 `analysis.json`，格式保持稳定：

```json
{
  "summary": "不超过 200 字",
  "facts": [
    {
      "id": "short-name",
      "claim": "关键事实",
      "details": "可选说明",
      "evidence": [{"path": "core/src/main/java/feign/Feign.java", "line": 217}]
    }
  ],
  "request_flow": ["..."],
  "extension_points": ["..."]
}
```

重点帮我说清楚：`Feign.builder().target(...)` 最后怎么创建代理；接口注解怎么被解析成方法元数据和请求模板；代理方法调用怎么分派到 handler；参数怎么填进 `RequestTemplate` 并叠加目标 URL；请求拦截器、编码器、HTTP Client、响应解码、错误解码和重试分别在链路的哪个位置。每个关键事实都要给真实 `path` 和 `line` 证据。
