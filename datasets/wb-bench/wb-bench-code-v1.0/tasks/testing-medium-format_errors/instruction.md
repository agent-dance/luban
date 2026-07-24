我试 Markup 的百分号插值时，mapping-like 参数缺 key、tuple 参数转义和百分号字面量这些地方都缺少稳定回归。自定义 mapping 跟百分号字面量混在一起也看一下。麻烦只补 tests，先别改 formatting.py 或包入口。测试环境按 unittest discover 跑，别加 pytest 依赖，十五到二十个边界用例就够。
