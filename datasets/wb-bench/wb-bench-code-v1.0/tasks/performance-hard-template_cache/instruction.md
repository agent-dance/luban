我这里有个模板环境会反复渲染同一批模板，现在 CPU 都花在 tokenize/compile 上了。麻烦把重复模板的开销压下来，autoescape、filters 和 cache_size 这些边界别弄乱。
