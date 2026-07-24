多分类评估脚本现在只看 overall accuracy，我排 badcase 时不够用。麻烦补一份更完整的 report：从每行 scores 里取 top1，算每个 label 的 precision/recall/f1/support，再给 macro、weighted 和 accuracy；同分的时候按 labels.json 里的顺序选，未知真实标签不要把脚本跑挂。
