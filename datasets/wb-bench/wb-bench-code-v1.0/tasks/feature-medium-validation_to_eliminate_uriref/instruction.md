URI 校验对某些非法 component 组合放得太宽，后面才会出更难懂的问题。麻烦给 `Validator` 加个能显式检查 component 合法性的用法，非法时抛清楚的 `InvalidComponentsError`。
