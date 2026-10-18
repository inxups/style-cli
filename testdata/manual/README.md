# 手动探针与离线检查

本目录保留 Codex / Claude 的隔离探针、采集器和测试辅助代码。历史文档、计数语料和个人 API 配置已移除。

从仓库根运行离线测试：

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover \
  -s testdata/manual -p 'test_*.py'
```

离线测试使用临时合成输入，不需要 API key，也不调用真实模型。真实探针需要另行提供已认证的隔离环境；查看各脚本的 `--help`。

`calibrate_tokens.py` 和 `compare_tokens_offline.py` 必须显式传入 `--corpus /absolute/path/to/corpus.json`。输入包含 `baseline_prefix`、`user_message` 和 `rules`；40 条规则需中英各半，每条包含唯一 `id`、`language`、`text` 和正文的 `sha256`。仓库不再提供默认校准语料。

`calibrate_tokens.py` 只读取进程环境中的 `OPENAI_API_KEY` / `ANTHROPIC_API_KEY`。缺少凭据时保留未完成状态，不调用计数接口。`compare_tokens_offline.py` 的可选 `tiktoken` 依赖仅用于手动编码对照，不属于产品或普通 CI 依赖。

重新生成 exact 计数参考也需要显式语料：

```sh
python3 scripts/generate_m3_exact_golden.py \
  --corpus /absolute/path/to/corpus.json \
  --output /tmp/exact-text-reference.json
```

`m1b-probes` 与 `claude-skills` 是探针 fixture，不会自动安装。计时和 usage 汇总脚本只读取显式提供的日志；输出不代表单条规则的官方计费。
