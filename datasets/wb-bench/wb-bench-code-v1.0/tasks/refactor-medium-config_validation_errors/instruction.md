mkdocs_like/config.py 这里是一个很小的配置校验器，validate_config 要继续返回 (cfg, errors)。我想把默认值填充、字段类型检查和错误收集拆清楚；errors 里继续用 path 列表和 message 字符串。site_name、theme、plugins、nav、extra 这几个字段的正常加载和错误路径/message 都别变。
