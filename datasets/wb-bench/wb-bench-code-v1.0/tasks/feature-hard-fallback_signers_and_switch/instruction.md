我们做 key rotation 时，希望新 signer 验不过还能尝试几套旧 signer。帮 Serializer 支持 `fallback_signers`，配置形式尽量跟现有 signer 参数风格贴近，最后的签名错误也别丢信息；fallback 配置处理最好能单独覆盖。把 fallback_signers 的主流程和单独覆盖入口也补到测试里。
