我这里想梳理一下 Jekyll 一次 `jekyll build` 到底怎么跑完的，不需要你改代码。

请在仓库根目录生成 `analysis.json`，格式保持稳定：

```json
{
  "summary": "不超过 200 字",
  "facts": [
    {
      "id": "short-name",
      "claim": "关键事实",
      "details": "可选说明",
      "evidence": [{"path": "lib/jekyll/site.rb", "line": 74}]
    }
  ],
  "build_flow": ["..."],
  "extension_points": ["..."]
}
```

重点帮我讲清楚：build 命令入口怎么拿配置并创建 `Site`；`Site#process` 的几个阶段各自做什么；Reader 怎么把 layouts、pages、posts、collections、static files、data 读进来；plugins、generators、converters 是在哪里接入的；Liquid、converter 和 layout 渲染的顺序；cleanup/write 怎么把内容写到 destination；hooks 在哪些阶段会被触发。每个关键事实都要给真实 `path` 和 `line` 证据。
