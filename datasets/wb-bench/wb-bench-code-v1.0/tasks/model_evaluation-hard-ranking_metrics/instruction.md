我这边离线评估推荐结果时发现 topK 指标不太可信，重复 item 和同分排序会把 recall、ndcg 带偏。麻烦把这份 ranking 评估脚本补完整，按用户算 precision/recall/ndcg/map，再汇总整体结果；排序和去重规则要稳定，别让同一批样本每次跑出来不一样。
