我想把几个稀疏类目特征做成稳定的 cross feature，现在脚本里遇到空值、大小写和未知组合时输出会变。麻烦把交叉特征这块补好：单字段先按 vocab 编码，country/device 和 plan/channel 再按 cross vocab 编码，缺失和没见过的组合都要走默认桶。
