我这边做特征筛选查询时，把 `QuerySet` 放进 `FilteredRelation` 后会直接抛 `AttributeError`，这类输入不应该让查询构造过程这样崩掉。麻烦处理一下这个场景，普通 relation 查询别受影响。
