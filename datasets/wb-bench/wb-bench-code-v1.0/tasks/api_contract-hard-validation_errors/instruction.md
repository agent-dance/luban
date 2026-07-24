调用方现在依赖 validation error 的结构做展示，但我们这边有些场景只给了字符串。麻烦把 errors() 的 loc/msg/type/input 这些字段稳定下来，alias、缺字段、额外字段和默认值都别乱。
