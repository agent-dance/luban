有些部署里我们想彻底禁用 `.env` 加载，不想改代码。帮 python-dotenv 加个 `PYTHON_DOTENV_DISABLED` 环境变量开关，打开后 `load_dotenv()` 直接不加载并返回 False；最好有个调试提示，README 也说一下这个开关。
