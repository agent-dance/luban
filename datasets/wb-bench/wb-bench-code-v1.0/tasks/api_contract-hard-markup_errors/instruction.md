markup 解析这块现在遇到坏标签时有时直接吞掉，调用方不好定位。麻烦把 parse/render/escape 的行为固定：正常嵌套能解析，未闭合和错配要抛带 code/position 的错误。
