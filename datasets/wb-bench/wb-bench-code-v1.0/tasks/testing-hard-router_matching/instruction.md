我手动跑路由匹配回归时，converter、strict slash 和 MethodNotAllowed.allowed 这些特别容易漏测。麻烦只补测试，覆盖 int/string/path、redirect、method allowed 和静态优先这些边界，router.py 不要改。这里用 unittest discover 跑，所以别写 pytest 风格，尽量补够十五到二十个小用例。
