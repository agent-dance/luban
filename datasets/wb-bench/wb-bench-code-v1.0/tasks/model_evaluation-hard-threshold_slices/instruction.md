我跑二分类评估时发现阈值表和分人群的结果对不上，尤其有 sample weight、空 segment 的时候更乱。帮我把 evaluator 补一下：几个阈值都要算混淆矩阵和 precision/recall/f1，还要能按 segment 拆开看，边界分数正好等于阈值时也别飘。
