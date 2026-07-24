嵌套对象或数组 item 里多了未知字段时，错误 path 指得不准，数据清洗那边不好定位。麻烦让错误 path 落到具体多出来的字段；schema_path 还是指到 additionalProperties 这条规则就行，别因为未知字段名不同而变来变去。报错文案先别顺手改。
