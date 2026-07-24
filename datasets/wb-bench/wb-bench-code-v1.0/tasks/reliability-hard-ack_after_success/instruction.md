线上 worker 偶尔遇到任务失败但消息已经 ack 掉，后面就没法重试了。帮我把 ack/requeue/dead-letter 的顺序理顺，成功才 ack，重试和失败路径也别重复确认。
