团队里已经有几处 nox session 在重复写“先装脚本声明的依赖、再跑脚本”这套流程，后面维护起来有点散。能不能给 `Session` 补一个 `install_and_run_script()` 入口，让大家直接跑带依赖声明的脚本；体验尽量沿用现在的 install/run，别让每个项目再各写一套。
