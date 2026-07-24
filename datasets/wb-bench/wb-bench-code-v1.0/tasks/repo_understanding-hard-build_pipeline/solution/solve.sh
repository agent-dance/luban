#!/usr/bin/env bash
set -euo pipefail

cat > analysis.json <<'JSON'
{
  "summary": "Jekyll build 从 Commands::Build 读取配置并创建 Site。Site#process 依次 reset、read、generate、render、cleanup、write；Reader 装载内容，插件/生成器/转换器在 setup 接入，最后写入 destination。",
  "facts": [
    {
      "id": "command_config_site",
      "claim": "Commands::Build.process 会调整日志、调用 configuration_from_options 读取 Jekyll.configuration，然后 new Jekyll::Site；build(site, options) 再通过 process_site 进入 Site#process。",
      "details": "Jekyll.configuration 会读取配置文件，并把 DEFAULTS、_config 和命令行 override 合并。",
      "evidence": [
        {"path": "lib/jekyll/commands/build.rb", "line": 25},
        {"path": "lib/jekyll/commands/build.rb", "line": 55},
        {"path": "lib/jekyll/command.rb", "line": 27},
        {"path": "lib/jekyll.rb", "line": 114}
      ]
    },
    {
      "id": "site_process_stages",
      "claim": "Site#process 是一次构建的主流水线，依次执行 reset、read、generate、render、cleanup、write。",
      "details": "reset 清空站点内存状态和 liquid/regenerator 缓存，后续阶段在这个干净状态上读入、生成、渲染并写出。",
      "evidence": [
        {"path": "lib/jekyll/site.rb", "line": 74},
        {"path": "lib/jekyll/site.rb", "line": 95}
      ]
    },
    {
      "id": "reader_ingest",
      "claim": "Reader.read 负责读取 layouts、递归目录、include/exclude、collections、theme assets 和 data；目录读取会把 posts、pages、static_files 分别装入 site。",
      "details": "read_directories 用 YAML front matter 区分页和静态文件，posts 由 PostReader 处理，data 由 DataReader 处理并可合并 theme data。",
      "evidence": [
        {"path": "lib/jekyll/reader.rb", "line": 14},
        {"path": "lib/jekyll/reader.rb", "line": 55},
        {"path": "lib/jekyll/reader.rb", "line": 88},
        {"path": "lib/jekyll/reader.rb", "line": 117},
        {"path": "lib/jekyll/reader.rb", "line": 128}
      ]
    },
    {
      "id": "collection_document_read",
      "claim": "Collection.read 遍历 collection 目录，带 YAML header 的文件通过 read_document 创建 Document 并读取 front matter/data，不带 header 的进 static file；Document.read 负责读内容和 post data，并用 published? 控制是否进入 docs。",
      "details": "这解释了 collection 文档、静态资源和 unpublished 过滤在读取阶段的关系。",
      "evidence": [
        {"path": "lib/jekyll/collection.rb", "line": 57},
        {"path": "lib/jekyll/collection.rb", "line": 217},
        {"path": "lib/jekyll/document.rb", "line": 299},
        {"path": "lib/jekyll/document.rb", "line": 290}
      ]
    },
    {
      "id": "plugins_generators_converters",
      "claim": "Site#setup 先让 plugin_manager.conscientious_require 加载插件，再 instantiate_subclasses(Jekyll::Converter) 和 instantiate_subclasses(Jekyll::Generator) 建出 converters 与 generators；Site#generate 逐个调用 generator.generate(site)。",
      "details": "PluginManager 会加载 theme deps、plugins_dir 下的 rb 文件和配置的 gem plugins，安全模式下还会做白名单判断。",
      "evidence": [
        {"path": "lib/jekyll/site.rb", "line": 128},
        {"path": "lib/jekyll/site.rb", "line": 190},
        {"path": "lib/jekyll/plugin_manager.rb", "line": 19},
        {"path": "lib/jekyll/generator.rb", "line": 4},
        {"path": "lib/jekyll/converter.rb", "line": 4}
      ]
    },
    {
      "id": "render_liquid_convert_layout",
      "claim": "Site#render 为 docs 和 pages 设置 payload 后调用 renderer.run；Renderer 先触发 pre_render，若需要则 render_liquid，然后 convert 经过匹配的 Converter，触发 post_convert，最后按 layout 嵌套渲染。",
      "details": "Renderer.render_liquid 通过 site.liquid_renderer.file(path).parse(content) 解析 Liquid，布局渲染时把 content 放进 payload。",
      "evidence": [
        {"path": "lib/jekyll/site.rb", "line": 203},
        {"path": "lib/jekyll/site.rb", "line": 568},
        {"path": "lib/jekyll/renderer.rb", "line": 52},
        {"path": "lib/jekyll/renderer.rb", "line": 70},
        {"path": "lib/jekyll/renderer.rb", "line": 123},
        {"path": "lib/jekyll/renderer.rb", "line": 150}
      ]
    },
    {
      "id": "cleanup_write_hooks",
      "claim": "Site#cleanup 调 Cleaner.cleanup! 清理 destination 里的 obsolete 文件并触发 clean:on_obsolete；Site#write 遍历 each_site_file，对 regenerator 判定需要更新的 item 调 write(dest)，最后写 metadata 并触发 site:post_write。Document/Page/StaticFile 各自实现实际写文件。",
      "details": "Hooks 模块维护 site/pages/documents/clean 等 owner 的事件表，trigger 会按优先级调用已注册 hook。",
      "evidence": [
        {"path": "lib/jekyll/site.rb", "line": 220},
        {"path": "lib/jekyll/site.rb", "line": 228},
        {"path": "lib/jekyll/cleaner.rb", "line": 14},
        {"path": "lib/jekyll/document.rb", "line": 277},
        {"path": "lib/jekyll/convertible.rb", "line": 222},
        {"path": "lib/jekyll/static_file.rb", "line": 102},
        {"path": "lib/jekyll/hooks.rb", "line": 56},
        {"path": "lib/jekyll/hooks.rb", "line": 96}
      ]
    }
  ],
  "build_flow": [
    "Commands::Build.process 读取配置并创建 Jekyll::Site",
    "Build.build 调 process_site，process_site 调 site.process",
    "Site#process: reset -> read -> generate -> render -> cleanup -> write",
    "Reader 读 layouts/pages/posts/collections/static files/data",
    "Generators 追加或修改站点内容",
    "Renderer 对每个需要重建的文档执行 Liquid、Converter、layout",
    "Cleaner 清理 destination，write 阶段写文件和 regenerator metadata"
  ],
  "extension_points": [
    "plugins_dir 和 plugins gem",
    "Jekyll::Generator",
    "Jekyll::Converter",
    "Liquid tags/filters",
    "Hooks.register",
    "layouts",
    "theme assets",
    "front matter defaults"
  ]
}
JSON
